package filter

import (
	"math"
	"testing"

	"github.com/biotools/vcfilt/internal/parser"
)

// helper: build a Record with explicit DP, AF, Qual (no FilterRaw).
func rec(dp, af, qual float64) parser.Record {
	return parser.Record{DP: dp, AF: af, Qual: qual}
}

// recWithFilter builds a Record that also has a FilterRaw column value.
func recWithFilter(dp, af, qual float64, filterCol string) parser.Record {
	return parser.Record{DP: dp, AF: af, Qual: qual, FilterRaw: []byte(filterCol)}
}

func nan() float64 { return math.NaN() }

func TestPass_AllFiltersDisabled(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, false)
	// Every record should pass when all filters are disabled.
	if !Pass(rec(0, 1.0, 0), cfg) {
		t.Error("expected pass with all filters disabled")
	}
	if !Pass(rec(nan(), nan(), nan()), cfg) {
		t.Error("expected pass (NaN values) with all filters disabled")
	}
}

func TestPass_DPMin(t *testing.T) {
	cfg := NewConfig(10, -1, -1, false)

	if Pass(rec(9, nan(), nan()), cfg) {
		t.Error("DP=9 should fail DP>=10")
	}
	if !Pass(rec(10, nan(), nan()), cfg) {
		t.Error("DP=10 should pass DP>=10")
	}
	if !Pass(rec(100, nan(), nan()), cfg) {
		t.Error("DP=100 should pass DP>=10")
	}
	// Missing DP when filter enabled → reject.
	if Pass(rec(nan(), nan(), nan()), cfg) {
		t.Error("missing DP should fail when DP filter is enabled")
	}
}

func TestPass_AFMax(t *testing.T) {
	cfg := NewConfig(-1, 0.01, -1, false)

	if Pass(rec(nan(), 0.011, nan()), cfg) {
		t.Error("AF=0.011 should fail AF<=0.01")
	}
	if !Pass(rec(nan(), 0.01, nan()), cfg) {
		t.Error("AF=0.01 should pass AF<=0.01")
	}
	if !Pass(rec(nan(), 0.001, nan()), cfg) {
		t.Error("AF=0.001 should pass AF<=0.01")
	}
	// Missing AF when filter enabled → reject.
	if Pass(rec(nan(), nan(), nan()), cfg) {
		t.Error("missing AF should fail when AF filter is enabled")
	}
}

func TestPass_QualMin(t *testing.T) {
	cfg := NewConfig(-1, -1, 30, false)

	if Pass(rec(nan(), nan(), 29.9), cfg) {
		t.Error("QUAL=29.9 should fail QUAL>=30")
	}
	if !Pass(rec(nan(), nan(), 30), cfg) {
		t.Error("QUAL=30 should pass QUAL>=30")
	}
	if !Pass(rec(nan(), nan(), 99.9), cfg) {
		t.Error("QUAL=99.9 should pass QUAL>=30")
	}
	// Missing QUAL (NaN) when filter enabled → reject.
	if Pass(rec(nan(), nan(), nan()), cfg) {
		t.Error("missing QUAL should fail when QUAL filter is enabled")
	}
}

func TestPass_AllFiltersEnabled(t *testing.T) {
	cfg := NewConfig(10, 0.01, 30, false)

	// All fields satisfy → pass.
	if !Pass(rec(15, 0.005, 40), cfg) {
		t.Error("expected pass when all thresholds satisfied")
	}
	// DP too low → fail.
	if Pass(rec(5, 0.005, 40), cfg) {
		t.Error("expected fail: DP too low")
	}
	// AF too high → fail.
	if Pass(rec(15, 0.05, 40), cfg) {
		t.Error("expected fail: AF too high")
	}
	// QUAL too low → fail.
	if Pass(rec(15, 0.005, 20), cfg) {
		t.Error("expected fail: QUAL too low")
	}
	// All missing → fail.
	if Pass(rec(nan(), nan(), nan()), cfg) {
		t.Error("expected fail: all fields missing")
	}
}

func TestNewConfig_EnabledFlags(t *testing.T) {
	cfg := NewConfig(10, 0.01, 30, false)
	if !cfg.DPMinEnabled {
		t.Error("DPMinEnabled should be true for dpMin=10")
	}
	if !cfg.AFMaxEnabled {
		t.Error("AFMaxEnabled should be true for afMax=0.01")
	}
	if !cfg.QualMinEnabled {
		t.Error("QualMinEnabled should be true for qualMin=30")
	}

	cfgOff := NewConfig(-1, -1, -1, false)
	if cfgOff.DPMinEnabled || cfgOff.AFMaxEnabled || cfgOff.QualMinEnabled {
		t.Error("all filters should be disabled for negative thresholds")
	}
}

// ── PassOnly (FILTER column) tests ──────────────────────────────────────────

func TestPass_PassOnly_AcceptsPASS(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, true)
	r := recWithFilter(nan(), nan(), nan(), "PASS")
	if !Pass(r, cfg) {
		t.Error("FILTER=PASS should pass when --pass-only is enabled")
	}
}

func TestPass_PassOnly_RejectsDot(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, true)
	// "." is treated as NOT PASS (conservative, matches bcftools view -f PASS).
	r := recWithFilter(nan(), nan(), nan(), ".")
	if Pass(r, cfg) {
		t.Error("FILTER='.' should be rejected when --pass-only is enabled")
	}
}

func TestPass_PassOnly_RejectsOtherFilters(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, true)
	for _, col := range []string{"LowQual", "LowDepth", "LowQual;LowDepth", ""} {
		r := recWithFilter(nan(), nan(), nan(), col)
		if Pass(r, cfg) {
			t.Errorf("FILTER=%q should be rejected when --pass-only is enabled", col)
		}
	}
}

func TestPass_PassOnly_NilFilterRaw_Rejected(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, true)
	// A record with nil FilterRaw (e.g. malformed line missing column 6) must be rejected.
	r := rec(nan(), nan(), nan()) // FilterRaw is nil
	if Pass(r, cfg) {
		t.Error("nil FilterRaw should be rejected when --pass-only is enabled")
	}
}

func TestPass_PassOnly_Disabled_AllFilterValuesPass(t *testing.T) {
	cfg := NewConfig(-1, -1, -1, false)
	// When PassOnly is disabled, FILTER column is ignored.
	for _, col := range []string{"PASS", ".", "LowQual", ""} {
		r := recWithFilter(nan(), nan(), nan(), col)
		if !Pass(r, cfg) {
			t.Errorf("FILTER=%q should pass when --pass-only is disabled", col)
		}
	}
}

func TestPass_PassOnly_CombinedWithOtherFilters(t *testing.T) {
	cfg := NewConfig(10, -1, 30, true)

	// PASS + satisfies all numeric thresholds → pass.
	r := recWithFilter(15, nan(), 40, "PASS")
	if !Pass(r, cfg) {
		t.Error("expected pass: PASS filter + DP/QUAL thresholds satisfied")
	}

	// PASS but DP too low → fail.
	r2 := recWithFilter(5, nan(), 40, "PASS")
	if Pass(r2, cfg) {
		t.Error("expected fail: PASS but DP too low")
	}

	// Non-PASS but numeric thresholds satisfied → fail (PASS check wins).
	r3 := recWithFilter(15, nan(), 40, "LowQual")
	if Pass(r3, cfg) {
		t.Error("expected fail: non-PASS filter even though numeric thresholds pass")
	}
}

func BenchmarkPass(b *testing.B) {
	cfg := NewConfig(10, 0.01, 30, false)
	r := rec(42, 0.005, 55)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Pass(r, cfg)
	}
}

func BenchmarkPass_PassOnly(b *testing.B) {
	cfg := NewConfig(10, 0.01, 30, true)
	r := recWithFilter(42, 0.005, 55, "PASS")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Pass(r, cfg)
	}
}
