// Package pipeline orchestrates the Reader → Parser → Filter → Writer data flow
// using Go goroutines and channels for parallel processing with deterministic output.
//
// Architecture (batch-parallel design):
//
//	┌────────┐  line batches   ┌───────────────────────────┐  ordered batches  ┌────────┐
//	│ Reader │ ──────────────► │  N worker goroutines       │ ────────────────► │ Writer │
//	└────────┘                 │  (parse + filter per batch)│                   └────────┘
//	                           └───────────────────────────┘
//
// Batch design rationale:
//   - Instead of sending one line per channel op, we send slices of batchSize lines.
//   - This amortises channel send overhead over many records, reducing contention.
//   - For GZ files: decompression stays sequential (gzip is not parallelisable) but
//     each decompressed batch is processed by all N workers concurrently.
//   - For plain VCF: batching also reduces per-line goroutine scheduling overhead.
//
// Deterministic output: each batch retains its base sequence number so the
// merger can re-order results from different workers into input order.
package pipeline

import (
	"container/heap"
	"fmt"
	"sync"

	"github.com/biotools/vcfilt/internal/filter"
	"github.com/biotools/vcfilt/internal/parser"
	"github.com/biotools/vcfilt/internal/stats"
	"github.com/biotools/vcfilt/internal/writer"
)

// batchSize controls how many lines are grouped per batch.
// Larger = less channel overhead, more latency in the merger.
// 2048 lines ≈ 200KB of typical VCF data — good balance.
const batchSize = 2048

// Config holds pipeline runtime parameters.
type Config struct {
	Threads    int
	FilterCfg  filter.Config
	InputPath  string
	OutputPath string
	ShowStats  bool
}

// ─── batch types ─────────────────────────────────────────────────────────────

// lineBatch is a group of raw lines with their base sequence number.
type lineBatch struct {
	baseSeq uint64
	lines   [][]byte
}

// resultBatch carries the filtered results for an entire batch.
type resultBatch struct {
	baseSeq uint64
	// passed[i] == true means lines[i] should be written.
	passed []bool
	lines  [][]byte
}

// ─── resultBatchHeap (min-heap by baseSeq) ───────────────────────────────────

type resultBatchHeap []resultBatch

func (h resultBatchHeap) Len() int            { return len(h) }
func (h resultBatchHeap) Less(i, j int) bool  { return h[i].baseSeq < h[j].baseSeq }
func (h resultBatchHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *resultBatchHeap) Push(x interface{}) { *h = append(*h, x.(resultBatch)) }
func (h *resultBatchHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// ─── Run ─────────────────────────────────────────────────────────────────────

// Run executes the full filter pipeline and returns any error.
// s must be initialised by the caller (stats.New()); Run does NOT call s.Stop().
func Run(cfg Config, s *stats.Stats) error {
	// ── Phase 1: collect all header lines up front ────────────────────────
	headers, err := collectHeaders(cfg.InputPath)
	if err != nil {
		return fmt.Errorf("read headers: %w", err)
	}

	// ── Phase 2: open writer and emit headers ─────────────────────────────
	w, err := writer.Open(cfg.OutputPath)
	if err != nil {
		return err
	}
	defer w.Close()

	for _, h := range headers {
		if err := w.WriteHeader(h); err != nil {
			return fmt.Errorf("write header: %w", err)
		}
	}

	// ── Phase 3: batch-parallel streaming pipeline ────────────────────────
	// Buffer depth: enough batches to keep all workers busy.
	bufDepth := cfg.Threads * 4
	if bufDepth < 16 {
		bufDepth = 16
	}

	batchCh := make(chan lineBatch, bufDepth)
	resultCh := make(chan resultBatch, bufDepth)

	writerErrCh := make(chan error, 1)

	// Start merger/writer goroutine first (drains resultCh continuously).
	go func() {
		writerErrCh <- mergeAndWrite(resultCh, w)
	}()

	// Start N worker goroutines.
	var wg sync.WaitGroup
	for i := 0; i < cfg.Threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runBatchWorker(batchCh, resultCh, cfg.FilterCfg, s.Counter)
		}()
	}

	// Close resultCh when all workers finish.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Stream data lines as batches from the file.
	if err := streamDataBatches(cfg.InputPath, batchCh); err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Wait for merger/writer to finish.
	if err := <-writerErrCh; err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// ─── collectHeaders ───────────────────────────────────────────────────────────

func collectHeaders(path string) ([][]byte, error) {
	f, err := openVCF(path)
	if err != nil {
		return nil, err
	}
	defer f.close()

	var headers [][]byte
	for {
		line, eof, err := f.readLine()
		if err != nil {
			return nil, err
		}
		if eof {
			break
		}
		if line[0] == '#' {
			cp := make([]byte, len(line))
			copy(cp, line)
			headers = append(headers, cp)
		} else {
			break
		}
	}
	return headers, nil
}

// ─── streamDataBatches ───────────────────────────────────────────────────────

// streamDataBatches reads data lines in batches and sends them to batchCh.
// Uses a pool of line-slice buffers to reduce allocations across batches.
func streamDataBatches(path string, batchCh chan<- lineBatch) error {
	defer close(batchCh)

	f, err := openVCF(path)
	if err != nil {
		return err
	}
	defer f.close()

	// Use the underlying buffered scanner for fast line reading.
	sc := f.getScanner()

	var seq uint64
	buf := make([][]byte, 0, batchSize)

	flushBatch := func() {
		if len(buf) == 0 {
			return
		}
		batchCh <- lineBatch{baseSeq: seq - uint64(len(buf)), lines: buf}
		buf = make([][]byte, 0, batchSize)
	}

	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		if raw[0] == '#' {
			continue // skip headers
		}
		// Copy line bytes (sc.Bytes reuses its buffer).
		cp := make([]byte, len(raw))
		copy(cp, raw)
		buf = append(buf, cp)
		seq++
		if len(buf) >= batchSize {
			flushBatch()
		}
	}
	flushBatch() // send final partial batch
	return sc.Err()
}

// ─── runBatchWorker ──────────────────────────────────────────────────────────

// runBatchWorker processes entire batches of lines, filtering each one.
func runBatchWorker(
	batchCh <-chan lineBatch,
	resultCh chan<- resultBatch,
	cfg filter.Config,
	ctr *stats.Counter,
) {
	for batch := range batchCh {
		n := len(batch.lines)
		passed := make([]bool, n)
		total := uint64(n)
		var passedCount uint64

		for i, line := range batch.lines {
			rec, ok := parser.ParseRecord(line)
			if ok && filter.Pass(rec, cfg) {
				passed[i] = true
				passedCount++
			}
		}

		ctr.Total.Add(int64(total))
		ctr.Passed.Add(int64(passedCount))
		ctr.Skipped.Add(int64(total - passedCount))

		resultCh <- resultBatch{
			baseSeq: batch.baseSeq,
			passed:  passed,
			lines:   batch.lines,
		}
	}
}

// ─── mergeAndWrite ───────────────────────────────────────────────────────────

// mergeAndWrite re-orders result batches by baseSeq (min-heap) and writes
// passing records in the original input order.
func mergeAndWrite(resultCh <-chan resultBatch, w *writer.Writer) error {
	h := &resultBatchHeap{}
	heap.Init(h)

	var nextSeq uint64

	flushReady := func() error {
		for h.Len() > 0 && (*h)[0].baseSeq == nextSeq {
			rb := heap.Pop(h).(resultBatch)
			nextSeq += uint64(len(rb.lines))
			for i, line := range rb.lines {
				if rb.passed[i] {
					if err := w.WriteRecord(line); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	for rb := range resultCh {
		heap.Push(h, rb)
		if err := flushReady(); err != nil {
			return err
		}
	}

	// Drain remaining batches.
	for h.Len() > 0 {
		rb := heap.Pop(h).(resultBatch)
		for i, line := range rb.lines {
			if rb.passed[i] {
				if err := w.WriteRecord(line); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
