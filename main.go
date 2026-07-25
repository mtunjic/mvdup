package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"
)

// errInterrupted signals that the run was stopped early by an interrupt
// signal (Ctrl+C). It is not a failure: the report and summary are still
// written for whatever work completed before the signal arrived.
var errInterrupted = errors.New("interrupted by user")

func main() {
	err := run()
	switch {
	case err == nil:
		return
	case errors.Is(err, errInterrupted):
		os.Exit(130) // conventional exit code for SIGINT
	default:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		src     = flag.String("src", "", "directory to scan for duplicate files (required)")
		dest    = flag.String("dest", "./duplicates", "directory to move duplicate files into")
		report  = flag.String("report", "mvdup_report.txt", "path to write the text report to")
		minSize = flag.Int64("min-size", 1, "ignore files smaller than this many bytes")
		dryRun  = flag.Bool("dry-run", false, "only scan and report; do not move any files")
		workers = flag.Int("workers", runtime.NumCPU(), "number of concurrent hashing workers")
		live    = flag.Bool("progress", false, "print live progress to stdout while scanning, hashing, and moving")
	)
	flag.Parse()

	if *src == "" {
		flag.Usage()
		return fmt.Errorf("-src is required")
	}

	srcAbs, err := filepath.Abs(*src)
	if err != nil {
		return fmt.Errorf("resolving -src: %w", err)
	}
	info, err := os.Stat(srcAbs)
	if err != nil {
		return fmt.Errorf("-src: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("-src %q is not a directory", srcAbs)
	}

	destAbs, err := filepath.Abs(*dest)
	if err != nil {
		return fmt.Errorf("resolving -dest: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop() // also cancels ctx, so pipelineDone (deferred after this) must close first

	pipelineDone := make(chan struct{})
	defer close(pipelineDone)

	go func() {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\ninterrupt received, finishing the current file and writing the report...")
		case <-pipelineDone:
		}
	}()

	started := time.Now()
	prog := newProgress(*live, os.Stdout)

	entries, bytesScanned, scanWarnings := scanFiles(ctx, srcAbs, destAbs, *minSize, prog)

	groups, hashWarnings := findDuplicates(ctx, entries, *workers, prog)

	var moveResults []MoveResult
	if len(groups) > 0 {
		if !*dryRun {
			if err := os.MkdirAll(destAbs, 0o755); err != nil {
				return fmt.Errorf("creating -dest: %w", err)
			}
		}
		moveResults = moveDuplicates(ctx, groups, srcAbs, destAbs, *dryRun, prog)
	}

	prog.Close() // wait for all queued progress lines to print before the summary
	if prog != nil {
		fmt.Println()
	}

	interrupted := ctx.Err() != nil

	rep := &Report{
		SrcRoot:      srcAbs,
		DestRoot:     destAbs,
		DryRun:       *dryRun,
		Interrupted:  interrupted,
		Started:      started,
		Duration:     time.Since(started),
		FilesScanned: len(entries),
		BytesScanned: bytesScanned,
		Groups:       groups,
		MoveResults:  moveResults,
		ScanWarnings: scanWarnings,
		HashWarnings: hashWarnings,
	}

	writeConsoleSummary(os.Stdout, rep)

	f, err := os.Create(*report)
	if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	defer f.Close()
	writeTextReport(f, rep)

	fmt.Printf("Full report written to %s\n", *report)

	if interrupted {
		return errInterrupted
	}
	return nil
}
