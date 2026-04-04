// vcfilt — high-performance VCF/VCF.GZ filter.
//
// Usage:
//
//	vcfilt filter --input <file> --output <file> [options]
//
// See README.md for full documentation.
package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/biotools/vcfilt/internal/filter"
	"github.com/biotools/vcfilt/internal/indexer"
	"github.com/biotools/vcfilt/internal/pipeline"
	"github.com/biotools/vcfilt/internal/stats"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ─── rootCmd ─────────────────────────────────────────────────────────────────

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "vcfilt",
		Short: "High-performance VCF variant filter",
		Long: `vcfilt — A lightweight, compiled CLI tool for filtering large VCF and
VCF.GZ files using streaming and parallel processing with minimal memory usage.`,
		Version: version,
	}
	root.AddCommand(filterCmd())
	return root
}

// ─── filterCmd ───────────────────────────────────────────────────────────────

func filterCmd() *cobra.Command {
	var (
		input     string
		output    string
		dpMin     float64
		afMax     float64
		qualMin   float64
		threads   int
		showStats bool
		doIndex   bool
		tabixSIF  string
	)

	cmd := &cobra.Command{
		Use:   "filter",
		Short: "Filter VCF records by DP, AF, and QUAL thresholds",
		Long: `Filter variants from a VCF or VCF.GZ file.

Filtering rules (all enabled filters must pass):
  --dp-min    Minimum sequencing depth (DP in INFO field)
  --af-max    Maximum allele frequency (AF in INFO field; minimum across alleles for multi-allelic sites)
  --qual-min  Minimum variant quality (QUAL column)

Records missing a required tag when its filter is enabled are rejected.

Indexing (optional):
  --index       bgzip-compress and tabix-index the output (requires sorted input)
  --tabix-sif   Path to Singularity SIF image containing bgzip/tabix
                (auto-detected from PATH if not set)

Example:
  vcfilt filter \
    --input  variants.vcf.gz \
    --output filtered.vcf \
    --dp-min  10 \
    --af-max  0.01 \
    --qual-min 30 \
    --threads  8 \
    --index \
    --tabix-sif /COLD_STORAGE/software/tools/tabix/tabix.sif \
    --stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFilter(input, output, dpMin, afMax, qualMin, threads, showStats, doIndex, tabixSIF)
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "Input VCF or VCF.GZ file (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output VCF file (required)")
	cmd.Flags().Float64Var(&dpMin, "dp-min", -1, "Minimum depth (DP); disabled if negative")
	cmd.Flags().Float64Var(&afMax, "af-max", math.NaN(), "Maximum allele frequency (AF); disabled if negative or NaN")
	cmd.Flags().Float64Var(&qualMin, "qual-min", -1, "Minimum quality (QUAL); disabled if negative")
	cmd.Flags().IntVar(&threads, "threads", runtime.NumCPU(), "Number of parallel filter workers")
	cmd.Flags().BoolVar(&showStats, "stats", false, "Print processing statistics to stderr after completion")
	cmd.Flags().BoolVar(&doIndex, "index", false, "bgzip + tabix-index the output VCF (requires sorted input)")
	cmd.Flags().StringVar(&tabixSIF, "tabix-sif", "", "Path to Singularity SIF image with bgzip/tabix (auto-detected if empty)")

	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// ─── runFilter ───────────────────────────────────────────────────────────────

func runFilter(
	input, output string,
	dpMin, afMax, qualMin float64,
	threads int,
	showStats bool,
	doIndex bool,
	tabixSIF string,
) error {
	// Validate threads.
	if threads < 1 {
		threads = 1
	}

	// Build filter configuration.
	filterCfg := filter.NewConfig(dpMin, afMax, qualMin)

	// Print active filter summary.
	printFilterSummary(filterCfg, threads, input, output, doIndex)

	// Initialise stats (starts the clock).
	s := stats.New()

	// Build and run the pipeline.
	pipeCfg := pipeline.Config{
		Threads:    threads,
		FilterCfg:  filterCfg,
		InputPath:  input,
		OutputPath: output,
		ShowStats:  showStats,
	}

	if err := pipeline.Run(pipeCfg, s); err != nil {
		return fmt.Errorf("pipeline error: %w", err)
	}

	// Stop the clock.
	s.Stop()

	// Always print a one-line completion summary.
	elapsed := s.Elapsed().Round(time.Millisecond)
	total := s.Counter.Total.Load()
	passed := s.Counter.Passed.Load()
	fmt.Fprintf(os.Stderr, "[vcfilt] Done — %d/%d variants passed in %s (%.0f var/s)\n",
		passed, total, elapsed, s.Throughput())

	// Optionally print full stats table.
	if showStats {
		s.Print(os.Stderr)
	}

	// ── Optional: bgzip + tabix index ────────────────────────────────────────
	if doIndex {
		fmt.Fprintln(os.Stderr, "[vcfilt] Indexing: bgzip + tabix …")

		// Resolve indexer configuration.
		var idxCfg indexer.Config
		if tabixSIF != "" {
			idxCfg = indexer.SingularityConfig(tabixSIF, threads)
		} else {
			bgzipCmd, tabixCmd, found := indexer.FindBgzipTabix("")
			if !found {
				return fmt.Errorf("--index: bgzip/tabix not found on PATH; use --tabix-sif to specify a Singularity image")
			}
			idxCfg = indexer.Config{
				BgzipBin: bgzipCmd,
				TabixBin: tabixCmd,
				Threads:  threads,
			}
		}

		gzPath, err := indexer.BgzipAndIndex(output, idxCfg)
		if err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[vcfilt] Index complete: %s  (+.tbi)\n", gzPath)
	}

	return nil
}

// ─── printFilterSummary ───────────────────────────────────────────────────────

func printFilterSummary(cfg filter.Config, threads int, input, output string, doIndex bool) {
	fmt.Fprintln(os.Stderr, "┌─────────────────────────────────────────┐")
	fmt.Fprintln(os.Stderr, "│           vcfilt — VCF Filter           │")
	fmt.Fprintln(os.Stderr, "└─────────────────────────────────────────┘")
	fmt.Fprintf(os.Stderr, "  Input  : %s\n", input)
	fmt.Fprintf(os.Stderr, "  Output : %s\n", output)
	fmt.Fprintf(os.Stderr, "  Threads: %d\n", threads)
	fmt.Fprintln(os.Stderr, "  Filters:")
	if cfg.DPMinEnabled {
		fmt.Fprintf(os.Stderr, "    DP   >= %.0f\n", cfg.DPMin)
	} else {
		fmt.Fprintln(os.Stderr, "    DP   : disabled")
	}
	if cfg.AFMaxEnabled {
		fmt.Fprintf(os.Stderr, "    AF   <= %.6f\n", cfg.AFMax)
	} else {
		fmt.Fprintln(os.Stderr, "    AF   : disabled")
	}
	if cfg.QualMinEnabled {
		fmt.Fprintf(os.Stderr, "    QUAL >= %.0f\n", cfg.QualMin)
	} else {
		fmt.Fprintln(os.Stderr, "    QUAL : disabled")
	}
	if doIndex {
		fmt.Fprintln(os.Stderr, "  Index  : bgzip + tabix (enabled)")
	}
	fmt.Fprintln(os.Stderr, "─────────────────────────────────────────")
}
