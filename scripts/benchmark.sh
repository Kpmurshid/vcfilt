#!/usr/bin/env bash
# benchmark.sh — vcfilt performance benchmark script
#
# Measures vcfilt throughput across thread counts and compares against
# bcftools (if installed). Results are printed in a markdown-friendly table.
#
# Usage:
#   bash scripts/benchmark.sh [--records N] [--threads-max N]
#
# Requirements:
#   - vcfilt binary at ./vcfilt (run: go build -o vcfilt ./cmd/vcfilt/)
#   - Python 3 (for test data generation)
#   - bcftools (optional, for comparison)
#   - hyperfine (optional, for precise benchmarking)

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
RECORDS=1000000
THREADS_MAX=$(nproc)
INPUT_VCF="testdata/bench.vcf"
INPUT_GZ="testdata/bench.vcf.gz"
OUTPUT_DIR="/tmp/vcfilt_bench"
DP_MIN=10
AF_MAX=0.01
QUAL_MIN=30

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --records)   RECORDS="$2";     shift 2 ;;
    --threads-max) THREADS_MAX="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

# ── Colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

banner() { echo -e "\n${BOLD}${CYAN}>>> $* ${RESET}"; }

# ── 1. Build binary ──────────────────────────────────────────────────────────
banner "Building vcfilt"
go build -ldflags="-s -w" -o vcfilt ./cmd/vcfilt/
echo -e "${GREEN}✓ Binary built${RESET}"

# ── 2. Generate test data ────────────────────────────────────────────────────
banner "Generating ${RECORDS} variant test file"
if [[ ! -f "$INPUT_VCF" ]]; then
  python3 scripts/gen_test_vcf.py --records "$RECORDS" --output "$INPUT_VCF"
else
  echo "  (using existing $INPUT_VCF)"
fi
if [[ ! -f "$INPUT_GZ" ]]; then
  python3 scripts/gen_test_vcf.py --records "$RECORDS" --output "$INPUT_GZ"
else
  echo "  (using existing $INPUT_GZ)"
fi
VCF_SIZE=$(du -sh "$INPUT_VCF" | cut -f1)
GZ_SIZE=$(du -sh "$INPUT_GZ" | cut -f1)
echo -e "${GREEN}✓ Plain VCF: ${VCF_SIZE}   GZ: ${GZ_SIZE}${RESET}"

# ── 3. vcfilt thread-scaling benchmark ──────────────────────────────────────
banner "vcfilt thread-scaling benchmark (plain VCF, ${RECORDS} records)"
echo ""
printf "%-12s %-12s %-14s %-14s\n" "Threads" "Wall time" "Throughput" "Speedup"
printf "%-12s %-12s %-14s %-14s\n" "-------" "---------" "----------" "-------"

BASE_TIME=""
for T in 1 2 4 "$THREADS_MAX"; do
  # Avoid duplicate entries if THREADS_MAX is already in the list
  [[ "$T" -gt "$THREADS_MAX" ]] && continue

  OUT="$OUTPUT_DIR/out_${T}t.vcf"
  START=$(date +%s%3N)
  ./vcfilt filter \
    --input    "$INPUT_VCF" \
    --output   "$OUT" \
    --dp-min   "$DP_MIN" \
    --af-max   "$AF_MAX" \
    --qual-min "$QUAL_MIN" \
    --threads  "$T" 2>/dev/null
  END=$(date +%s%3N)

  ELAPSED_MS=$(( END - START ))
  ELAPSED_S=$(echo "scale=3; $ELAPSED_MS/1000" | bc)
  THROUGHPUT=$(echo "scale=0; $RECORDS * 1000 / $ELAPSED_MS" | bc)

  if [[ -z "$BASE_TIME" ]]; then
    BASE_TIME="$ELAPSED_MS"
    SPEEDUP="1.00x"
  else
    SPEEDUP=$(echo "scale=2; $BASE_TIME / $ELAPSED_MS" | bc)"x"
  fi

  printf "%-12s %-12s %-14s %-14s\n" "${T}" "${ELAPSED_S}s" "${THROUGHPUT} var/s" "$SPEEDUP"
done

# ── 4. GZ input benchmark ────────────────────────────────────────────────────
banner "vcfilt GZ input benchmark (${THREADS_MAX} threads)"
OUT="$OUTPUT_DIR/out_gz.vcf"
START=$(date +%s%3N)
./vcfilt filter \
  --input    "$INPUT_GZ" \
  --output   "$OUT" \
  --dp-min   "$DP_MIN" \
  --af-max   "$AF_MAX" \
  --qual-min "$QUAL_MIN" \
  --threads  "$THREADS_MAX" 2>/dev/null
END=$(date +%s%3N)
ELAPSED_MS=$(( END - START ))
ELAPSED_S=$(echo "scale=3; $ELAPSED_MS/1000" | bc)
THROUGHPUT=$(echo "scale=0; $RECORDS * 1000 / $ELAPSED_MS" | bc)
echo "  GZ input: ${ELAPSED_S}s  |  ${THROUGHPUT} var/s"

# ── 5. Correctness check ─────────────────────────────────────────────────────
banner "Correctness: 1-thread vs ${THREADS_MAX}-thread output"
if diff <(grep -v "^#" "$OUTPUT_DIR/out_1t.vcf") \
        <(grep -v "^#" "$OUTPUT_DIR/out_${THREADS_MAX}t.vcf") > /dev/null 2>&1; then
  echo -e "${GREEN}✓ Outputs are identical (deterministic)${RESET}"
else
  echo -e "\033[0;31m✗ Outputs differ — non-deterministic output detected!\033[0m"
  exit 1
fi

# ── 6. bcftools comparison (optional) ────────────────────────────────────────
if command -v bcftools &>/dev/null; then
  banner "bcftools comparison"
  BC_OUT="$OUTPUT_DIR/bcftools_out.vcf"
  START=$(date +%s%3N)
  bcftools view \
    -i "INFO/DP>=${DP_MIN} && INFO/AF<=${AF_MAX} && QUAL>=${QUAL_MIN}" \
    -o "$BC_OUT" \
    "$INPUT_VCF" 2>/dev/null
  END=$(date +%s%3N)
  BC_MS=$(( END - START ))
  BC_S=$(echo "scale=3; $BC_MS/1000" | bc)
  BC_TP=$(echo "scale=0; $RECORDS * 1000 / $BC_MS" | bc)
  VCFILT_MS=$BASE_TIME
  RATIO=$(echo "scale=2; $BC_MS / $VCFILT_MS" | bc)

  printf "\n%-16s %-12s %-14s\n" "Tool" "Wall time" "Throughput"
  printf "%-16s %-12s %-14s\n" "----" "---------" "----------"
  printf "%-16s %-12s %-14s\n" "vcfilt (1t)"   "$(echo "scale=3; $VCFILT_MS/1000" | bc)s" "$(echo "scale=0; $RECORDS*1000/$VCFILT_MS" | bc) var/s"
  printf "%-16s %-12s %-14s\n" "bcftools"       "${BC_S}s"       "${BC_TP} var/s"
  echo ""
  echo -e "  vcfilt is ${RATIO}x the speed of bcftools (1-thread)"

  # Record-count correctness vs bcftools
  VC_COUNT=$(grep -v "^#" "$OUTPUT_DIR/out_1t.vcf" | wc -l)
  BC_COUNT=$(grep -v "^#" "$BC_OUT" | wc -l)
  echo "  vcfilt passed: $VC_COUNT   bcftools passed: $BC_COUNT"
  if [[ "$VC_COUNT" == "$BC_COUNT" ]]; then
    echo -e "${GREEN}✓ Record counts match${RESET}"
  else
    echo -e "\033[0;33m⚠ Record counts differ (may be due to bcftools missing/absent filter semantics)${RESET}"
  fi
else
  echo ""
  echo "  (bcftools not found — skipping comparison; install with: sudo apt-get install bcftools)"
fi

# ── 7. hyperfine precise benchmark (optional) ───────────────────────────────
if command -v hyperfine &>/dev/null; then
  banner "hyperfine precise benchmark"
  hyperfine --warmup 1 \
    "./vcfilt filter --input $INPUT_VCF --output /tmp/hf_out.vcf --dp-min $DP_MIN --af-max $AF_MAX --qual-min $QUAL_MIN --threads 1" \
    "./vcfilt filter --input $INPUT_VCF --output /tmp/hf_out.vcf --dp-min $DP_MIN --af-max $AF_MAX --qual-min $QUAL_MIN --threads $THREADS_MAX" \
    2>&1
fi

banner "Benchmark complete"
echo "  Output files in: $OUTPUT_DIR"
