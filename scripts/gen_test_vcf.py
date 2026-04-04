#!/usr/bin/env python3
"""
gen_test_vcf.py — Generate synthetic VCF test data for vcfilt benchmarking.

Usage:
    python3 scripts/gen_test_vcf.py --records 1000000 --output testdata/large.vcf
    python3 scripts/gen_test_vcf.py --records 1000000 --output testdata/large.vcf.gz --gzip

The generated file follows the VCF 4.2 spec with realistic DP, AF, and QUAL
distributions that exercise all three filter thresholds.
"""
import argparse
import gzip
import random
import sys


HEADER = """\
##fileformat=VCFv4.2
##FILTER=<ID=PASS,Description="All filters passed">
##INFO=<ID=DP,Number=1,Type=Integer,Description="Total Depth">
##INFO=<ID=AF,Number=A,Type=Float,Description="Allele Frequency">
##INFO=<ID=MQ,Number=1,Type=Float,Description="RMS Mapping Quality">
##INFO=<ID=BaseQRankSum,Number=1,Type=Float,Description="Z-score from Wilcoxon rank sum test">
##FORMAT=<ID=GT,Number=1,Type=String,Description="Genotype">
##FORMAT=<ID=DP,Number=1,Type=Integer,Description="Approximate read depth">
##FORMAT=<ID=GQ,Number=1,Type=Integer,Description="Genotype Quality">
##contig=<ID=chr1,length=248956422>
##contig=<ID=chr2,length=242193529>
##contig=<ID=chr3,length=198295559>
#CHROM\tPOS\tID\tREF\tALT\tQUAL\tFILTER\tINFO\tFORMAT\tSAMPLE1"""

CHROMS = ["chr1", "chr2", "chr3", "chr4", "chr5"]
BASES  = ["A", "C", "G", "T"]


def rand_alt(ref, rng):
    return rng.choice([b for b in BASES if b != ref])


def make_record(chrom, pos, rng):
    ref = rng.choice(BASES)
    alt = rand_alt(ref, rng)
    dp  = rng.randint(1, 200)        # depth 1–200, filter typically >=10
    af  = round(rng.uniform(0, 0.5), 6)  # AF 0–0.5, filter typically <=0.01
    mq  = round(rng.uniform(20, 60), 2)
    qual = round(rng.uniform(0, 100), 2)  # QUAL 0–100, filter typically >=30
    bqrs = round(rng.uniform(-3, 3), 3)
    gt   = rng.choice(["0/1", "1/1", "0/0"])
    sample_dp = rng.randint(1, dp)
    gq   = rng.randint(0, 99)

    info = f"DP={dp};AF={af};MQ={mq};BaseQRankSum={bqrs}"
    fmt  = "GT:DP:GQ"
    sample = f"{gt}:{sample_dp}:{gq}"

    return f"{chrom}\t{pos}\t.\t{ref}\t{alt}\t{qual}\tPASS\t{info}\t{fmt}\t{sample}"


def main():
    ap = argparse.ArgumentParser(description="Generate synthetic VCF for vcfilt benchmarking")
    ap.add_argument("--records", type=int, default=100_000,
                    help="Number of variant records to generate (default: 100000)")
    ap.add_argument("--output", default="testdata/test.vcf",
                    help="Output file path (default: testdata/test.vcf)")
    ap.add_argument("--gzip", action="store_true",
                    help="Write gzip-compressed output")
    ap.add_argument("--seed", type=int, default=42,
                    help="Random seed for reproducibility (default: 42)")
    args = ap.parse_args()

    rng = random.Random(args.seed)
    open_fn = gzip.open if args.gzip or args.output.endswith(".gz") else open
    mode    = "wt"

    n = args.records
    print(f"[gen_test_vcf] Generating {n:,} records → {args.output}", file=sys.stderr)

    with open_fn(args.output, mode) as fh:
        fh.write(HEADER + "\n")

        chrom = CHROMS[0]
        pos   = 10_000
        for i in range(n):
            # Advance chromosome roughly every n/5 records.
            if i > 0 and i % (n // len(CHROMS)) == 0:
                cidx  = min(i // (n // len(CHROMS)), len(CHROMS) - 1)
                chrom = CHROMS[cidx]
                pos   = 10_000

            pos += rng.randint(1, 500)
            fh.write(make_record(chrom, pos, rng) + "\n")

    print(f"[gen_test_vcf] Done.", file=sys.stderr)


if __name__ == "__main__":
    main()
