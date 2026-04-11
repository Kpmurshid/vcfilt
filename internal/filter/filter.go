// Package filter implements the threshold-based VCF record filtering logic.
// All comparisons are performed on pre-parsed numeric values or raw byte slices;
// no string operations happen in the hot path.
package filter

import (
	"math"

	"github.com/biotools/vcfilt/internal/parser"
)

// passBytes is the exact byte sequence expected in the FILTER column for a
// passing record. Declared once at package level — never reallocated.
var passBytes = []byte("PASS")

// Config holds the filter thresholds. Use math.NaN() or the sentinel
// constants to disable a particular filter.
type Config struct {
	// DPMin is the minimum sequencing depth. Records with DP < DPMin are
	// rejected. A value of 0 (default) disables this filter.
	DPMin float64

	// AFMax is the maximum allele frequency. Records with AF > AFMax are
	// rejected. A value of math.NaN() disables this filter.
	AFMax float64

	// QualMin is the minimum QUAL score. Records with QUAL < QualMin are
	// rejected. A value of 0 (default) disables this filter.
	QualMin float64

	// DPMinEnabled, AFMaxEnabled, QualMinEnabled control whether each filter
	// is active. Set automatically by NewConfig.
	DPMinEnabled   bool
	AFMaxEnabled   bool
	QualMinEnabled bool

	// PassOnly, when true, keeps only records where FILTER == "PASS".
	//
	// Design decision — FILTER="." handling:
	//   "." means the site has not been filtered (variant callers use it when
	//   no filter was applied). This is semantically different from "PASS",
	//   which means the site was evaluated and passed all filters.
	//   vcfilt conservatively EXCLUDES "." when --pass-only is set, matching
	//   bcftools view -f PASS behaviour. Users who want to keep "." variants
	//   should not use --pass-only, or should pre-process their VCF.
	PassOnly bool
}

// NewConfig constructs a Config from CLI-supplied values.
// Pass negative values to disable the corresponding numeric filter.
// passOnly enables the FILTER==PASS check.
func NewConfig(dpMin, afMax, qualMin float64, passOnly bool) Config {
	return Config{
		DPMin:          dpMin,
		AFMax:          afMax,
		QualMin:        qualMin,
		DPMinEnabled:   dpMin >= 0,
		AFMaxEnabled:   !math.IsNaN(afMax) && afMax >= 0,
		QualMinEnabled: qualMin >= 0,
		PassOnly:       passOnly,
	}
}

// Pass returns true if the record satisfies all enabled filter thresholds.
//
// Evaluation order (all must pass — AND semantics):
//  1. FILTER == "PASS"  (only when PassOnly is true)
//  2. DP >= DPMin       (only when DPMinEnabled)
//  3. AF <= AFMax       (only when AFMaxEnabled)
//  4. QUAL >= QualMin   (only when QualMinEnabled)
//
// When a required tag is absent (NaN) and the corresponding filter is enabled,
// the record is rejected (conservative behaviour matching bcftools -e).
func Pass(r parser.Record, cfg Config) bool {
	// ── 1. FILTER column ──────────────────────────────────────────────────
	if cfg.PassOnly {
		// FilterRaw is a zero-copy slice into the raw line set by the parser.
		// A nil slice means the column was absent (malformed line) → reject.
		if !bytesEqual(r.FilterRaw, passBytes) {
			return false
		}
	}

	// ── 2. INFO/DP ────────────────────────────────────────────────────────
	if cfg.DPMinEnabled {
		if math.IsNaN(r.DP) || r.DP < cfg.DPMin {
			return false
		}
	}

	// ── 3. INFO/AF ────────────────────────────────────────────────────────
	if cfg.AFMaxEnabled {
		if math.IsNaN(r.AF) || r.AF > cfg.AFMax {
			return false
		}
	}

	// ── 4. QUAL ───────────────────────────────────────────────────────────
	// QUAL="." is stored as NaN by the parser, so it is rejected here when
	// --qual-min is active (conservative, matches bcftools behaviour).
	if cfg.QualMinEnabled {
		if math.IsNaN(r.Qual) || r.Qual < cfg.QualMin {
			return false
		}
	}

	return true
}

// bytesEqual compares a byte slice to another byte slice without allocating.
// Used to compare FilterRaw against passBytes in the hot path.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
