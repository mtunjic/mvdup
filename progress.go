package main

import (
	"fmt"
	"io"
	"time"
)

// progressThrottle bounds how often the scan/hash ticker lines are rewritten,
// so scanning a huge tree doesn't spend most of its time flushing stdout.
const progressThrottle = 100 * time.Millisecond

type msgKind int

const (
	msgScanTick msgKind = iota
	msgScanDone
	msgHashTick
	msgMove
)

type progressMsg struct {
	kind msgKind

	count int   // msgScanTick: files found so far
	bytes int64 // msgScanTick: bytes found so far

	done, total int // msgHashTick: files hashed so far / total candidates

	from, to string
	dryRun   bool
	err      error
}

// Progress prints live updates to stdout as scanning, hashing, and moving
// happen.
type Progress struct {
	ch   chan progressMsg
	done chan struct{} // closed once the printer goroutine has drained ch
}

// newProgress starts a printer goroutine writing to out, or returns nil if
// enabled is false.
func newProgress(enabled bool, out io.Writer) *Progress {
	if !enabled {
		return nil
	}
	p := &Progress{
		ch:   make(chan progressMsg, 256),
		done: make(chan struct{}),
	}
	go p.printLoop(out)
	return p
}

// Close signals that no more messages will be sent and blocks until the
// printer goroutine has finished writing everything already queued. Callers
// must not send any more messages after calling Close.
func (p *Progress) Close() {
	if p == nil {
		return
	}
	close(p.ch)
	<-p.done
}

// ScanTick is called as each file is discovered during the walk.
func (p *Progress) ScanTick(count int, bytes int64) {
	if p == nil {
		return
	}
	p.ch <- progressMsg{kind: msgScanTick, count: count, bytes: bytes}
}

// ScanDone prints the final scan tally and ends the scanning line.
func (p *Progress) ScanDone() {
	if p == nil {
		return
	}
	p.ch <- progressMsg{kind: msgScanDone}
}

// HashTick is called after each candidate file has been hashed. Safe to call
// concurrently from multiple hashing workers.
func (p *Progress) HashTick(done, total int) {
	if p == nil {
		return
	}
	p.ch <- progressMsg{kind: msgHashTick, done: done, total: total}
}

// Move reports the outcome of relocating (or, in dry-run mode, the plan to
// relocate) a single duplicate file.
func (p *Progress) Move(from, to string, dryRun bool, err error) {
	if p == nil {
		return
	}
	p.ch <- progressMsg{kind: msgMove, from: from, to: to, dryRun: dryRun, err: err}
}

// printLoop is the sole owner of all printing state (last-print timestamps,
// running scan tallies).
func (p *Progress) printLoop(out io.Writer) {
	defer close(p.done)

	var lastScanPrint, lastHashPrint time.Time
	var scannedCount int
	var scannedBytes int64

	for msg := range p.ch {
		switch msg.kind {
		case msgScanTick:
			scannedCount = msg.count
			scannedBytes = msg.bytes
			if time.Since(lastScanPrint) < progressThrottle {
				continue
			}
			lastScanPrint = time.Now()
			fmt.Fprintf(out, "\rscanning... %d files found (%s)", scannedCount, humanBytes(scannedBytes))

		case msgScanDone:
			fmt.Fprintf(out, "\rscanned %d files (%s)                    \n", scannedCount, humanBytes(scannedBytes))

		case msgHashTick:
			final := msg.done >= msg.total
			if !final && time.Since(lastHashPrint) < progressThrottle {
				continue
			}
			lastHashPrint = time.Now()
			fmt.Fprintf(out, "\rhashing... %d/%d candidate files", msg.done, msg.total)
			if final {
				fmt.Fprintln(out)
			}

		case msgMove:
			if msg.err != nil {
				fmt.Fprintf(out, "FAILED:     %s -> %s (%v)\n", msg.from, msg.to, msg.err)
				continue
			}
			verb := "moved:     "
			if msg.dryRun {
				verb = "would move:"
			}
			fmt.Fprintf(out, "%s %s -> %s\n", verb, msg.from, msg.to)
		}
	}
}
