// Package reader provides buffered, streaming VCF input with transparent
// gzip decompression. It separates header lines (starting with '#') from
// data lines and sends them through separate channels.
package reader

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// bufSize is the read buffer size — 1 MiB balances syscall overhead vs
	// memory usage for large files.
	bufSize = 1 << 20 // 1 MiB
)

// LineReader streams a VCF file line-by-line.
type LineReader struct {
	scanner *bufio.Scanner
	closer  io.Closer // underlying gzip.Reader, if any
	file    *os.File
}

// Open opens a VCF or VCF.GZ file and returns a LineReader.
// The caller must call Close() when done.
func Open(path string) (*LineReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}

	var raw io.Reader = f
	var gzCloser io.Closer

	if isGzip(path) {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip open %q: %w", path, err)
		}
		raw = gz
		gzCloser = gz
	}

	sc := bufio.NewScanner(raw)
	// Increase the scanner buffer to handle very long VCF lines (e.g. large INFO fields).
	buf := make([]byte, bufSize)
	sc.Buffer(buf, 16*bufSize)

	return &LineReader{
		scanner: sc,
		closer:  gzCloser,
		file:    f,
	}, nil
}

// isGzip checks the file extension to determine if gzip decompression is needed.
func isGzip(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".gz")
}

// StreamLines reads the VCF file and sends header lines and data lines to
// separate channels. It closes both channels when EOF is reached or an error
// occurs.
//
//   - headers: receives raw header lines (starting with '#'), in order.
//   - records: receives raw data line bytes (without newline).
//
// The function blocks until the file is fully consumed; run it in a goroutine.
func (lr *LineReader) StreamLines(headers chan<- []byte, records chan<- []byte) error {
	defer close(headers)
	defer close(records)

	for lr.scanner.Scan() {
		// scanner.Bytes() returns a slice backed by the internal buffer;
		// copy it so callers own independent memory.
		raw := lr.scanner.Bytes()
		line := make([]byte, len(raw))
		copy(line, raw)

		if len(line) == 0 {
			continue
		}

		if line[0] == '#' {
			headers <- line
		} else {
			records <- line
		}
	}

	if err := lr.scanner.Err(); err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	return nil
}

// Close releases file handles.
func (lr *LineReader) Close() error {
	if lr.closer != nil {
		if err := lr.closer.Close(); err != nil {
			lr.file.Close()
			return err
		}
	}
	return lr.file.Close()
}
