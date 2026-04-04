// Package stats tracks processing counters and timing for benchmarking.
package stats

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// Counter holds lock-free atomic counters updated by worker goroutines.
type Counter struct {
	Total   atomic.Int64 // total records seen
	Passed  atomic.Int64 // records that passed all filters
	Skipped atomic.Int64 // records that failed at least one filter
}

// Stats aggregates processing results with timing.
type Stats struct {
	Counter   *Counter
	StartTime time.Time
	EndTime   time.Time
}

// New creates a new Stats with an initialised counter and records the start time.
func New() *Stats {
	return &Stats{
		Counter:   &Counter{},
		StartTime: time.Now(),
	}
}

// Stop records the end time.
func (s *Stats) Stop() {
	s.EndTime = time.Now()
}

// Elapsed returns the wall-clock duration of processing.
func (s *Stats) Elapsed() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

// Throughput returns variants processed per second (total, not just passed).
func (s *Stats) Throughput() float64 {
	secs := s.Elapsed().Seconds()
	if secs == 0 {
		return 0
	}
	return float64(s.Counter.Total.Load()) / secs
}

// Print writes a human-readable stats summary to w.
func (s *Stats) Print(w io.Writer) {
	total := s.Counter.Total.Load()
	passed := s.Counter.Passed.Load()
	skipped := s.Counter.Skipped.Load()
	elapsed := s.Elapsed()
	throughput := s.Throughput()

	fmt.Fprintln(w, "─────────────────────────────────────────")
	fmt.Fprintf(w, "  Total variants processed : %d\n", total)
	fmt.Fprintf(w, "  Variants passed filters  : %d\n", passed)
	fmt.Fprintf(w, "  Variants filtered out    : %d\n", skipped)
	if total > 0 {
		fmt.Fprintf(w, "  Pass rate                : %.2f%%\n",
			float64(passed)/float64(total)*100)
	}
	fmt.Fprintf(w, "  Wall-clock time          : %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(w, "  Throughput               : %.0f variants/sec\n", throughput)
	fmt.Fprintln(w, "─────────────────────────────────────────")
}
