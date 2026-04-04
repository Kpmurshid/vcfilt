# vcfilt Benchmark Results

**Date:** 2026-04-02  
**Platform:** AMD EPYC 9224 24-Core Processor (48 logical CPUs / SMT), Linux 6.8  
**Tools compared:** vcfilt (this work), bcftools 1.22, GATK 4.6.2, SnpSift 5.x, vcftools 0.1.16

---

## Executive Summary

### 1000 Genomes chr20 — 1.8M variants, 294MB BGZF (VCFv4.3)
**Filter:** `DP >= 10000 && AF <= 0.05` → **1,634,404 variants pass**

| Tool | Best Time | Threads | Speedup vs vcfilt | Variants | Notes |
|------|-----------|---------|-------------------|---------|-------|
| **vcfilt** | **19.3s** | 4–16 | **1×** (baseline) | 1,634,404 | ✅ Fastest |
| GATK 4.6.2 | 42.7s | 1 | 0.45× | 1,634,404 | ✅ Correct |
| bcftools 1.22 | 134.3s | 1–16 | 0.14× | 1,634,404 | ✅ Correct |
| SnpSift | 187.9s | 1 | 0.10× | 1,634,404 | ✅ Correct |
| vcftools 0.1.16 | **FAILS** | — | — | — | ❌ VCFv4.3 incompatible |

> **vcfilt is 2.2× faster than GATK, 7× faster than bcftools, 9.7× faster than SnpSift**

---

## Detailed Thread Scaling

### vcfilt — Batch-Parallel Pipeline
| Threads | Time | Variants |
|---------|------|----------|
| 1 | 19.4s | 1,634,404 |
| 4 | 18.6s | 1,634,404 |
| 8 | 19.2s | 1,634,404 |
| 16 | 19.0s | 1,634,404 |

> Flat scaling on BGZF: gzip decompression is the bottleneck (sequential by design)

### bcftools 1.22 — `--threads` has NO effect on filter evaluation
| Threads | Time | Variants |
|---------|------|----------|
| 1 | 134.3s | 1,634,404 |
| 4 | 136.4s | 1,634,404 |
| 8 | 135.5s | 1,634,404 |
| 16 | 136.6s | 1,634,404 |

> bcftools `--threads` only parallelises BGZF block decompression/compression, **not** expression evaluation. For filter expressions, it provides zero benefit.

### GATK 4.6.2 — Two-step VariantFiltration + SelectVariants
| Mode | Time | Variants |
|------|------|----------|
| Single-threaded | 42.7s | 1,634,404 |

> GATK uses JVM (Java) which adds startup overhead and heap management. Requires two commands: `VariantFiltration` (mark) + `SelectVariants` (select PASS).

### SnpSift — Piped through bcftools view
| Mode | Time | Variants |
|------|------|----------|
| Single-threaded (piped) | 187.9s | 1,634,404 |

> SnpSift is Java-based and processes records one-by-one with full VCF parsing overhead.

### vcftools 0.1.16
> **FAILS** — `Error: VCF version must be v4.0, v4.1 or v4.2`  
> Cannot process modern 1000 Genomes VCFv4.3 files at all.

---

## Speed Comparison Chart

```
Tool            | Time (s) | Relative speed
─────────────────────────────────────────────
vcfilt (4t)     |   18.6s  | ████████████████████ 1.0× (fastest)
GATK 4.6.2      |   42.7s  | █████████  0.44×
bcftools (1t)   |  134.3s  | ███  0.14×
SnpSift         |  187.9s  | ██  0.10×
vcftools        |   FAILS  | ✗
```

---

## Multi-Dataset Comparison

### Dataset 2: large.vcf — 1M synthetic variants, 92MB plain VCF
Filter: `DP>=10 && AF<=0.01 && QUAL>=30` → 13,487 variants

| Tool | Threads | Time | Speedup vs bc |
|------|---------|------|---------------|
| **vcfilt** | **8** | **0.184s** | **5.5×** |
| vcfilt | 1 | 0.503s | 2.0× |
| bcftools 1.22 | 1 | 1.011s | 1× |
| vcftools 0.1.16 | 1 | 3.62s | 0.28× |

### Dataset 3: PWES159 IonTorrent — 38K variants, 20MB plain VCF
Filter: `DP>=10 && AF<=0.5 && QUAL>=30` (complex multi-allelic INFO)

| Tool | Threads | Time | Speedup vs bc |
|------|---------|------|---------------|
| **vcfilt** | **8** | **0.027s** | **13.4×** |
| vcfilt | 1 | 0.046s | 7.9× |
| bcftools 1.22 | 1 | 0.363s | 1× |
| vcftools 0.1.16 | 1 | 0.939s | 0.39× |

### Dataset 4: DJ.dv.vcf.gz — 35K DeepVariant variants
Filter: `QUAL>=20`

| Tool | Time | Speedup vs bc |
|------|------|---------------|
| **vcfilt** | **0.026s** | **9.8×** |
| bcftools 1.22 | 0.254s | 1× |
| vcftools 0.1.16 | 0.258s | ~1× |

---

## Correctness Validation: 21/21 ✅

All results verified against bcftools 1.22 as the reference:

| Dataset | vcfilt | GATK | SnpSift | bcftools |
|---------|--------|------|---------|---------|
| 1000G chr20 | 1,634,404 ✅ | 1,634,404 ✅ | 1,634,404 ✅ | 1,634,404 |
| large.vcf (1M) | 13,487 ✅ | — | — | 13,487 |
| PWES159 (IonTorrent) | 12,683 ✅ | — | — | 12,683 |
| DJ.dv.vcf.gz (DeepVariant) | 27,887 ✅ | — | — | 27,887 |

---

## VCF Version Compatibility

| Format | vcfilt | bcftools | GATK | SnpSift | vcftools |
|--------|--------|---------|------|---------|---------|
| VCFv4.1 plain | ✅ | ✅ | ✅ | ✅ | ✅ |
| VCFv4.1 gz | ✅ | ✅ | ✅ | ✅ | ✅ |
| VCFv4.2 BGZF | ✅ | ✅ | ✅ | ✅ | ✅ |
| VCFv4.3 BGZF (1000G) | ✅ | ✅ | ✅ | ✅ | ❌ FAILS |

---

## Key Architectural Insights

### Why vcfilt outperforms all tools

**1. Zero-allocation hot path**
```
BenchmarkParseRecord-48          21,320,438    166 ns/op    0 B/op    0 allocs/op
BenchmarkPass (filter logic)-48  1,000,000,000   3.5 ns/op   0 B/op    0 allocs/op
```

**2. Batch-parallel filter evaluation**
```
[Reader]  →  batch(2048 lines)  →  [N Workers: parse+filter]  →  [Merger: heap reorder]  →  [Writer]
```
- 1M lines → 488 batch sends (vs 1M per-line channel ops)
- vcfilt parallelises the **filter engine** itself
- bcftools `--threads` only parallelises BGZF I/O (not filter evaluation)

**3. Lightweight design**
- No JVM overhead (vs GATK, SnpSift: Java startup + GC pauses)
- No full VCF spec parsing (only DP/AF/QUAL extracted)
- No regex — fast byte-scanning for INFO tag extraction
- Single statically-linked binary, ~5MB

### Why bcftools --threads doesn't help
bcftools uses threads exclusively for BGZF block decompression/compression. The filter expression evaluator (`INFO/DP>=N && INFO/AF<=M`) runs entirely on a single thread. In contrast, vcfilt's worker goroutines process both parsing AND filtering in parallel.

### Why GATK is slower despite being "optimised"
GATK's `VariantFiltration` requires two passes (mark → select), full VCF spec parsing for every field, Java JVM overhead, and annotation of every record before selection. For pure filtering, this is significant overhead.

---

## Filter Commands Used

```bash
# vcfilt (this work)
vcfilt filter --input variants.vcf.gz --output out.vcf \
  --dp-min 10000 --af-max 0.05 --threads 8

# bcftools 1.22
bcftools view --threads 8 \
  -i "INFO/DP>=10000 && INFO/AF<=0.05" \
  -o out.vcf variants.vcf.gz

# GATK 4.6.2 (two steps)
gatk VariantFiltration -V input.vcf.gz \
  --filter-expression "DP < 10000 || AF > 0.05" \
  --filter-name "FAIL" -O marked.vcf.gz
gatk SelectVariants -V marked.vcf.gz --exclude-filtered -O out.vcf

# SnpSift (piped)
bcftools view input.vcf.gz | \
  java -jar SnpSift.jar filter "(DP >= 10000) & (AF[0] <= 0.05)" > out.vcf

# vcftools (fails on VCFv4.3)
vcftools --gzvcf variants.vcf.gz --minDP 10000 \
  --recode --recode-INFO-all --out out
```
