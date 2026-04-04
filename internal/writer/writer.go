// Package writer provides a buffered, thread-safe VCF output writer.
// Records are written in the order they are received from the ordering channel,
// ensuring deterministic (position-ordered) output even under parallel filtering.
package writer

import (
	"bufio"
	"fmt"
	"os"
)

const writerBufSize = 1 << 20 // 1 MiB output buffer

// Writer wraps a buffered file writer for VCF output.
type Writer struct {
	bw   *bufio.Writer
	file *os.File
}

// Open creates or truncates the output file and returns a Writer.
// The caller must call Close() to flush and release the file.
func Open(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create output %q: %w", path, err)
	}
	return &Writer{
		bw:   bufio.NewWriterSize(f, writerBufSize),
		file: f,
	}, nil
}

// WriteHeader writes a VCF header line followed by a newline.
func (w *Writer) WriteHeader(line []byte) error {
	if _, err := w.bw.Write(line); err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

// WriteRecord writes a VCF data line followed by a newline.
func (w *Writer) WriteRecord(line []byte) error {
	if _, err := w.bw.Write(line); err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

// Flush flushes the internal buffer to the underlying OS file.
func (w *Writer) Flush() error {
	return w.bw.Flush()
}

// Close flushes and closes the output file.
func (w *Writer) Close() error {
	if err := w.bw.Flush(); err != nil {
		w.file.Close()
		return fmt.Errorf("flush: %w", err)
	}
	return w.file.Close()
}
