// Package indexer provides bgzip compression and tabix indexing of VCF output.
//
// After vcfilt writes a plain VCF, the indexer can:
//  1. bgzip-compress it → output.vcf.gz
//  2. tabix-index it   → output.vcf.gz.tbi
//
// Both bgzip and tabix are invoked as subprocesses. The caller supplies the
// path to the bgzip/tabix binary (or singularity exec wrapper).
// The VCF must be sorted by CHROM+POS for tabix to succeed.
package indexer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Config holds indexer settings.
type Config struct {
	// BgzipBin is the path/command for bgzip (e.g. "bgzip" or
	// "singularity exec /path/tabix.sif bgzip").
	// If empty, the indexer looks for bgzip on PATH.
	BgzipBin string

	// TabixBin is the path/command for tabix.
	// If empty, looks for tabix on PATH.
	TabixBin string

	// Threads passed to bgzip --threads (if supported).
	Threads int
}

// DefaultConfig returns a Config that uses bgzip/tabix from PATH.
func DefaultConfig(threads int) Config {
	return Config{
		BgzipBin: "bgzip",
		TabixBin: "tabix",
		Threads:  threads,
	}
}

// SingularityConfig returns a Config that wraps bgzip/tabix inside a
// Singularity container image.
func SingularityConfig(sifPath string, threads int) Config {
	return Config{
		BgzipBin: fmt.Sprintf("singularity exec %s bgzip", sifPath),
		TabixBin: fmt.Sprintf("singularity exec %s tabix", sifPath),
		Threads:  threads,
	}
}

// BgzipAndIndex bgzip-compresses plainVCF → plainVCF+".gz", then
// runs tabix to create plainVCF+".gz.tbi".
// The original plain VCF is removed after successful compression.
//
// Returns the path to the compressed file.
func BgzipAndIndex(plainVCF string, cfg Config) (string, error) {
	gzPath := plainVCF + ".gz"

	// ── Step 1: bgzip ────────────────────────────────────────────────────────
	if err := runBgzip(plainVCF, gzPath, cfg); err != nil {
		return "", fmt.Errorf("bgzip: %w", err)
	}

	// Remove plain VCF after successful compression.
	if err := os.Remove(plainVCF); err != nil {
		// Non-fatal — the .gz file is the important output.
		fmt.Fprintf(os.Stderr, "[vcfilt] warning: could not remove plain VCF: %v\n", err)
	}

	// ── Step 2: tabix ────────────────────────────────────────────────────────
	if err := runTabix(gzPath, cfg); err != nil {
		return gzPath, fmt.Errorf("tabix: %w", err)
	}

	return gzPath, nil
}

// runBgzip compresses src → dst using bgzip.
func runBgzip(src, dst string, cfg Config) error {
	// bgzip -c --threads N src > dst
	// -c writes to stdout so we can redirect to dst.
	parts := splitCmd(cfg.BgzipBin)
	args := append(parts[1:],
		"-c",
		"--threads", fmt.Sprintf("%d", max(cfg.Threads, 1)),
		src,
	)
	cmd := exec.Command(parts[0], args...)

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(dst) // clean up partial file
		return fmt.Errorf("bgzip exited: %w", err)
	}
	return nil
}

// runTabix indexes a bgzip-compressed VCF.
func runTabix(gzPath string, cfg Config) error {
	// tabix -p vcf gzPath
	parts := splitCmd(cfg.TabixBin)
	args := append(parts[1:], "-p", "vcf", gzPath)
	cmd := exec.Command(parts[0], args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr // tabix prints progress to stdout sometimes

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tabix exited: %w", err)
	}
	return nil
}

// splitCmd splits a command string (e.g. "singularity exec foo.sif bgzip")
// into argv components.
func splitCmd(s string) []string {
	return strings.Fields(s)
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FindBgzipTabix auto-detects bgzip and tabix on PATH or in common
// Singularity image locations. Returns (bgzipCmd, tabixCmd, found).
func FindBgzipTabix(singularitySIF string) (bgzip, tabix string, found bool) {
	// Prefer system binaries.
	if p, err := exec.LookPath("bgzip"); err == nil {
		if t, err2 := exec.LookPath("tabix"); err2 == nil {
			return p, t, true
		}
	}

	// Fall back to Singularity container if provided.
	if singularitySIF != "" {
		if _, err := os.Stat(singularitySIF); err == nil {
			return fmt.Sprintf("singularity exec %s bgzip", singularitySIF),
				fmt.Sprintf("singularity exec %s tabix", singularitySIF),
				true
		}
	}

	return "", "", false
}
