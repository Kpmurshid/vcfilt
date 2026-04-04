package parser

import (
	"math"
	"testing"
)

// ── ParseRecord tests ────────────────────────────────────────────────────────

func TestParseRecord_FullLine(t *testing.T) {
	line := []byte("chr1\t12345\t.\tA\tT\t55.3\tPASS\tDP=42;AF=0.005;MQ=40.0\tGT:DP\t0/1:42")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true for valid line")
	}
	if r.DP != 42 {
		t.Errorf("DP: want 42, got %v", r.DP)
	}
	if r.AF != 0.005 {
		t.Errorf("AF: want 0.005, got %v", r.AF)
	}
	if math.Abs(r.Qual-55.3) > 1e-9 {
		t.Errorf("Qual: want 55.3, got %v", r.Qual)
	}
	if string(r.Raw) != string(line) {
		t.Error("Raw should be the original line")
	}
}

func TestParseRecord_MissingDP(t *testing.T) {
	line := []byte("chr1\t1\t.\tA\tG\t30.0\tPASS\tAF=0.01;MQ=50\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !math.IsNaN(r.DP) {
		t.Errorf("DP should be NaN when absent, got %v", r.DP)
	}
	if r.AF != 0.01 {
		t.Errorf("AF: want 0.01, got %v", r.AF)
	}
}

func TestParseRecord_MissingAF(t *testing.T) {
	line := []byte("chr1\t1\t.\tA\tG\t30.0\tPASS\tDP=100\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.DP != 100 {
		t.Errorf("DP: want 100, got %v", r.DP)
	}
	if !math.IsNaN(r.AF) {
		t.Errorf("AF should be NaN when absent, got %v", r.AF)
	}
}

func TestParseRecord_DotQual(t *testing.T) {
	line := []byte("chr1\t1\t.\tA\tG\t.\tPASS\tDP=50;AF=0.001\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !math.IsNaN(r.Qual) {
		t.Errorf("Qual should be NaN for '.', got %v", r.Qual)
	}
}

func TestParseRecord_MultiAllelicAF(t *testing.T) {
	// AF=0.3,0.1 — vcfilt stores the MINIMUM AF across all alleles so that
	// the af-max filter passes when ANY allele satisfies AF <= threshold,
	// matching bcftools semantics (INFO/AF<=X is true if any allele passes).
	line := []byte("chr1\t100\t.\tA\tG,T\t50.0\tPASS\tDP=20;AF=0.3,0.1\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.AF != 0.1 {
		t.Errorf("AF: want 0.1 (min of 0.3,0.1), got %v", r.AF)
	}

	// Also verify: all same value
	line2 := []byte("chr1\t200\t.\tA\tG,T\t50.0\tPASS\tDP=20;AF=0.5,0.5\tGT\t0/1")
	r2, ok2 := ParseRecord(line2)
	if !ok2 {
		t.Fatal("expected ok=true for line2")
	}
	if r2.AF != 0.5 {
		t.Errorf("AF: want 0.5 (min of 0.5,0.5), got %v", r2.AF)
	}

	// Single allele — unchanged behaviour
	line3 := []byte("chr1\t300\t.\tA\tG\t50.0\tPASS\tDP=20;AF=0.7\tGT\t0/1")
	r3, ok3 := ParseRecord(line3)
	if !ok3 {
		t.Fatal("expected ok=true for line3")
	}
	if r3.AF != 0.7 {
		t.Errorf("AF: want 0.7, got %v", r3.AF)
	}
}

func TestParseRecord_TooFewColumns(t *testing.T) {
	line := []byte("chr1\t1\t.\tA\tG\t40.0\tPASS") // only 7 cols (0-6), missing INFO
	_, ok := ParseRecord(line)
	if ok {
		t.Error("expected ok=false for line with <8 columns")
	}
}

func TestParseRecord_InfoFlagOnly(t *testing.T) {
	// INFO contains only flags, no DP or AF key=val tags.
	line := []byte("chr1\t1\t.\tA\tG\t60.0\tPASS\tSOMEFLAG;ANOTHERFLAG\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !math.IsNaN(r.DP) {
		t.Errorf("DP should be NaN (flag-only INFO), got %v", r.DP)
	}
	if !math.IsNaN(r.AF) {
		t.Errorf("AF should be NaN (flag-only INFO), got %v", r.AF)
	}
}

func TestParseRecord_DotInfo(t *testing.T) {
	// INFO = "." (missing) — common in real VCFs.
	line := []byte("chr1\t1\t.\tA\tG\t60.0\tPASS\t.\tGT\t0/1")
	r, ok := ParseRecord(line)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !math.IsNaN(r.DP) {
		t.Errorf("DP should be NaN for '.' INFO, got %v", r.DP)
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

func BenchmarkParseRecord(b *testing.B) {
	line := []byte("chr1\t12345\t.\tA\tT\t55.3\tPASS\tDP=42;AF=0.005;MQ=40.0;BaseQRankSum=-1.23\tGT:DP:GQ\t0/1:42:99")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ParseRecord(line)
	}
}

func BenchmarkParseRecord_LargeInfo(b *testing.B) {
	// Simulate a record with a large INFO field (many tags).
	line := []byte("chr1\t12345\t.\tA\tT\t55.3\tPASS\t" +
		"AC=1;AF=0.005;AN=2;BaseQRankSum=-1.23;ClippingRankSum=0.0;DP=42;" +
		"ExcessHet=3.01;FS=0.0;MLEAC=1;MLEAF=0.005;MQ=40.0;MQRankSum=0.0;" +
		"QD=1.32;ReadPosRankSum=-0.5;SOR=0.7\tGT:AD:DP:GQ:PL\t0/1:20,22:42:99:630,0,584")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ParseRecord(line)
	}
}
