// Package filter implements the threshold-based VCF record filtering logic.
// All comparisons are performed on pre-parsed numeric values; no string
// operations happen here.
package filter

import (
	"math"

	"github.com/biotools/vcfilt/internal/parser"
)

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
}

// NewConfig constructs a Config from CLI-supplied values.
// Pass negative values to disable the corresponding filter.
func NewConfig(dpMin, afMax, qualMin float64) Config {
	return Config{
		DPMin:          dpMin,
		AFMax:          afMax,
		QualMin:        qualMin,
		DPMinEnabled:   dpMin >= 0,
		AFMaxEnabled:   !math.IsNaN(afMax) && afMax >= 0,
		QualMinEnabled: qualMin >= 0,
	}
}

// Pass returns true if the record satisfies all enabled filter thresholds.
// When a required tag is absent (NaN) and the corresponding filter is enabled,
// the record is rejected (conservative behaviour matching bcftools -e).
func Pass(r parser.Record, cfg Config) bool {
	if cfg.DPMinEnabled {
		if math.IsNaN(r.DP) || r.DP < cfg.DPMin {
			return false
		}
	}

	if cfg.AFMaxEnabled {
		if math.IsNaN(r.AF) || r.AF > cfg.AFMax {
			return false
		}
	}

	if cfg.QualMinEnabled {
		if math.IsNaN(r.Qual) || r.Qual < cfg.QualMin {
			return false
		}
	}

	return true
}
