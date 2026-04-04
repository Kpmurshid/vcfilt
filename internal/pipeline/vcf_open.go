// vcf_open.go provides the low-level VCF file opening and line-reading
// helpers used by streamFile in pipeline.go.
// This file exists so the pipeline package can do its own streaming without
// introducing an import cycle with the reader package.
package pipeline

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

const scanBufSize = 1 << 20 // 1 MiB per scanner buffer

// vcfFile wraps a buffered scanner over a plain or gzip-compressed VCF file.
type vcfFile struct {
	scanner *bufio.Scanner
	gz      *gzip.Reader
	file    *os.File
}

// openVCF is assigned as a function variable so it can be swapped in tests.
var openVCF = func(path string) (*vcfFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}

	var raw io.Reader = f
	var gz *gzip.Reader

	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gz, err = gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip reader %q: %w", path, err)
		}
		raw = gz
	}

	sc := bufio.NewScanner(raw)
	buf := make([]byte, scanBufSize)
	sc.Buffer(buf, 16*scanBufSize)

	return &vcfFile{scanner: sc, gz: gz, file: f}, nil
}

// readLine returns the next non-empty line as an owned []byte, a bool
// indicating EOF, and any scan error.
// On EOF it returns (nil, true, nil).
func (v *vcfFile) readLine() ([]byte, bool, error) {
	for {
		if !v.scanner.Scan() {
			if err := v.scanner.Err(); err != nil {
				return nil, false, fmt.Errorf("scan: %w", err)
			}
			return nil, true, nil // EOF
		}
		raw := v.scanner.Bytes()
		if len(raw) == 0 {
			continue // skip blank lines
		}
		// Copy so the caller owns the memory (scanner reuses its buffer).
		line := make([]byte, len(raw))
		copy(line, raw)
		return line, false, nil
	}
}

// getScanner returns the underlying bufio.Scanner for direct batch reading.
func (v *vcfFile) getScanner() *bufio.Scanner {
	return v.scanner
}

// close releases all underlying resources.
func (v *vcfFile) close() {
	if v.gz != nil {
		v.gz.Close()
	}
	v.file.Close()
}
