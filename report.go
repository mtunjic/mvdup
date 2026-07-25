package main

import (
	"fmt"
	"io"
	"time"
)

// Report bundles everything needed to render the console summary and the
// text report file.
type Report struct {
	SrcRoot       string
	DestRoot      string
	DryRun        bool
	Interrupted   bool // true if the user stopped the run early (Ctrl+C)
	Started       time.Time
	Duration      time.Duration
	FilesScanned  int
	BytesScanned  int64
	Groups        []DupGroup
	MoveResults   []MoveResult
	ScanWarnings  []string
	HashWarnings  []string
}

func (r *Report) duplicateFileCount() int {
	n := 0
	for _, g := range r.Groups {
		n += len(g.Files) - 1
	}
	return n
}

func (r *Report) reclaimableBytes() int64 {
	var n int64
	for _, g := range r.Groups {
		n += g.Size * int64(len(g.Files)-1)
	}
	return n
}

func (r *Report) movedOK() int {
	n := 0
	for _, m := range r.MoveResults {
		if m.Err == nil {
			n++
		}
	}
	return n
}

func (r *Report) moveErrors() int {
	return len(r.MoveResults) - r.movedOK()
}

// writeConsoleSummary prints a short human-readable summary to w (typically stdout).
func writeConsoleSummary(w io.Writer, r *Report) {
	fmt.Fprintln(w, "==== mvdup summary ====")
	fmt.Fprintf(w, "Source:            %s\n", r.SrcRoot)
	fmt.Fprintf(w, "Destination:       %s\n", r.DestRoot)
	if r.DryRun {
		fmt.Fprintln(w, "Mode:              dry-run (no files moved)")
	}
	if r.Interrupted {
		fmt.Fprintln(w, "Status:            interrupted by user -- results below are partial")
	}
	fmt.Fprintf(w, "Duration:          %s\n", r.Duration.Round(time.Millisecond))
	fmt.Fprintf(w, "Files scanned:     %d (%s)\n", r.FilesScanned, humanBytes(r.BytesScanned))
	fmt.Fprintf(w, "Duplicate groups:  %d\n", len(r.Groups))
	fmt.Fprintf(w, "Duplicate files:   %d\n", r.duplicateFileCount())
	fmt.Fprintf(w, "Reclaimable space: %s\n", humanBytes(r.reclaimableBytes()))
	if !r.DryRun {
		fmt.Fprintf(w, "Files moved:       %d\n", r.movedOK())
		if r.moveErrors() > 0 {
			fmt.Fprintf(w, "Move errors:       %d\n", r.moveErrors())
		}
	}
	if n := len(r.ScanWarnings) + len(r.HashWarnings); n > 0 {
		fmt.Fprintf(w, "Warnings:          %d (see report file for details)\n", n)
	}
	fmt.Fprintln(w, "========================")
}

// writeTextReport writes the full, detailed report to w (typically a file).
func writeTextReport(w io.Writer, r *Report) {
	fmt.Fprintln(w, "mvdup duplicate file report")
	fmt.Fprintf(w, "Generated:         %s\n", r.Started.Format(time.RFC1123))
	fmt.Fprintf(w, "Source:            %s\n", r.SrcRoot)
	fmt.Fprintf(w, "Destination:       %s\n", r.DestRoot)
	fmt.Fprintf(w, "Dry run:           %v\n", r.DryRun)
	if r.Interrupted {
		fmt.Fprintln(w, "Status:            interrupted by user -- results below are partial")
	}
	fmt.Fprintf(w, "Duration:          %s\n", r.Duration.Round(time.Millisecond))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Files scanned:     %d (%s)\n", r.FilesScanned, humanBytes(r.BytesScanned))
	fmt.Fprintf(w, "Duplicate groups:  %d\n", len(r.Groups))
	fmt.Fprintf(w, "Duplicate files:   %d\n", r.duplicateFileCount())
	fmt.Fprintf(w, "Reclaimable space: %s\n", humanBytes(r.reclaimableBytes()))
	if !r.DryRun {
		fmt.Fprintf(w, "Files moved:       %d\n", r.movedOK())
		fmt.Fprintf(w, "Move errors:       %d\n", r.moveErrors())
	}
	fmt.Fprintln(w)

	if len(r.Groups) > 0 {
		moveByFrom := make(map[string]*MoveResult, len(r.MoveResults))
		for i := range r.MoveResults {
			moveByFrom[r.MoveResults[i].From] = &r.MoveResults[i]
		}

		fmt.Fprintln(w, "---- Duplicate groups ----")
		for i, g := range r.Groups {
			fmt.Fprintf(w, "\n[%d] sha256=%s  size=%s  copies=%d\n", i+1, g.Hash, humanBytes(g.Size), len(g.Files))
			fmt.Fprintf(w, "  kept:   %s\n", g.Files[0].Path)
			for _, dup := range g.Files[1:] {
				line := moveByFrom[dup.Path]
				switch {
				case line == nil:
					fmt.Fprintf(w, "  dup:    %s\n", dup.Path)
				case line.Err != nil:
					fmt.Fprintf(w, "  FAILED: %s -> %s (%v)\n", line.From, line.To, line.Err)
				case r.DryRun:
					fmt.Fprintf(w, "  would move: %s -> %s\n", line.From, line.To)
				default:
					fmt.Fprintf(w, "  moved:  %s -> %s\n", line.From, line.To)
				}
			}
		}
		fmt.Fprintln(w)
	}

	if len(r.ScanWarnings)+len(r.HashWarnings) > 0 {
		fmt.Fprintln(w, "---- Warnings ----")
		for _, wmsg := range r.ScanWarnings {
			fmt.Fprintln(w, " -", wmsg)
		}
		for _, wmsg := range r.HashWarnings {
			fmt.Fprintln(w, " -", wmsg)
		}
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
