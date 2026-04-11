// Package parser provides lightweight VCF record parsing.
// It avoids full INFO field parsing — only DP and AF tags are extracted
// using a fast byte-scanning approach (no regex).
package parser

import (
	"math"
	"strconv"
)

// Record holds the parsed fields of a single VCF data line.
// We keep the raw line to avoid re-serialising when writing output.
type Record struct {
	// Raw is the original unmodified line (no newline).
	Raw []byte

	// FilterRaw is the raw byte slice of the FILTER column (column 6, 0-based).
	// It points directly into Raw — zero allocation, no copy.
	// A nil slice means the column was not reached (malformed line).
	FilterRaw []byte

	// Parsed scalar fields.
	// Qual is math.NaN() when the QUAL column is "." or unparseable.
	Qual float64

	// INFO-extracted values; NaN means "not present".
	DP float64
	AF float64
}

// NaN sentinel used when a tag is absent.
var nanVal = math.NaN()

// passBytesLiteral is the expected byte content of a passing FILTER column.
// Declared as a package-level var so it is allocated once.
var passBytesLiteral = []byte("PASS")

// ParseRecord parses a raw VCF data line (must NOT start with '#').
// It fills only the fields needed for filtering; everything else stays in Raw.
// Returns false when the line is malformed (fewer than 8 tab-separated columns).
//
// Column extraction (0-based):
//   - 5 → QUAL  (float or "." → NaN)
//   - 6 → FILTER (raw slice into Raw; no copy)
//   - 7 → INFO  (DP= and AF= tags extracted)
func ParseRecord(line []byte) (Record, bool) {
	r := Record{
		Raw: line,
		DP:  nanVal,
		AF:  nanVal,
	}

	// Walk the line with a manual tab counter to avoid allocations.
	// We need columns 5 (QUAL), 6 (FILTER), and 7 (INFO).
	col := 0
	start := 0

	for i := 0; i <= len(line); i++ {
		if i == len(line) || line[i] == '\t' {
			switch col {
			case 5: // QUAL — float or "." → NaN
				seg := line[start:i]
				if len(seg) == 1 && seg[0] == '.' {
					// "." means missing QUAL — treated as NaN so that
					// --qual-min will reject this record (conservative).
					r.Qual = math.NaN()
				} else {
					q, err := strconv.ParseFloat(bytesToString(seg), 64)
					if err == nil {
						r.Qual = q
					} else {
						// Unparseable QUAL → NaN → rejected when --qual-min is active.
						r.Qual = math.NaN()
					}
				}
			case 6: // FILTER — keep a zero-copy slice into Raw
				// FilterRaw points into the original line buffer; no allocation.
				r.FilterRaw = line[start:i]
			case 7: // INFO — extract DP= and AF=, then stop
				extractInfoTags(line[start:i], &r)
				// INFO is the last column we care about — stop early.
				return r, true
			}
			col++
			start = i + 1
		}
	}

	// We need at least 8 columns (0–7). After the loop, col equals the number
	// of fields we saw (each field increments col after processing). A 7-field
	// line ends with col==7, so we need col >= 8 to have seen INFO (col 7).
	if col < 8 {
		return r, false
	}
	return r, true
}

// extractInfoTags scans the INFO field for DP=… and AF=… without allocating.
// The INFO field format: KEY=VAL;KEY=VAL;FLAG;…
func extractInfoTags(info []byte, r *Record) {
	start := 0
	for i := 0; i <= len(info); i++ {
		if i == len(info) || info[i] == ';' {
			tag := info[start:i]
			parseTag(tag, r)
			start = i + 1
		}
	}
}

// parseTag parses a single KEY=VAL or FLAG entry.
func parseTag(tag []byte, r *Record) {
	// Find '='
	eq := -1
	for j, b := range tag {
		if b == '=' {
			eq = j
			break
		}
	}
	if eq < 0 {
		return // flag, no value
	}
	key := tag[:eq]
	val := tag[eq+1:]

	switch {
	case bytesEqual(key, "DP"):
		// DP can be integer or float in some callers
		// Try integer first (faster), fall back to float.
		if iv, err := strconv.ParseInt(bytesToString(val), 10, 64); err == nil {
			r.DP = float64(iv)
		} else if fv, err := strconv.ParseFloat(bytesToString(val), 64); err == nil {
			r.DP = fv
		}
	case bytesEqual(key, "AF"):
		// AF can be comma-separated list (multi-allelic).
		// Store the MINIMUM AF across all alleles so that the AF filter
		// (af-max) passes when ANY allele satisfies AF <= threshold.
		// This matches bcftools behaviour: INFO/AF<=X is true if any
		// allele's AF satisfies the condition.
		minAF := math.NaN()
		start := 0
		for k := 0; k <= len(val); k++ {
			if k == len(val) || val[k] == ',' {
				seg := val[start:k]
				if fv, err := strconv.ParseFloat(bytesToString(seg), 64); err == nil {
					if math.IsNaN(minAF) || fv < minAF {
						minAF = fv
					}
				}
				start = k + 1
			}
		}
		r.AF = minAF
	}
}

// bytesEqual compares a byte slice to a string literal without allocation.
func bytesEqual(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// bytesToString converts []byte → string without copying (unsafe trick avoided;
// using stdlib cast which the compiler optimises to zero-copy in most paths).
func bytesToString(b []byte) string {
	return string(b)
}
