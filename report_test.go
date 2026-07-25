package main

import (
	"errors"
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReportCounts(t *testing.T) {
	r := &Report{
		Groups: []DupGroup{
			{Size: 100, Files: []FileEntry{{Path: "kept1"}, {Path: "dup1"}, {Path: "dup2"}}},
			{Size: 50, Files: []FileEntry{{Path: "kept2"}, {Path: "dup3"}}},
		},
		MoveResults: []MoveResult{
			{From: "dup1", To: "moved1"},
			{From: "dup2", To: "moved2", Err: errors.New("permission denied")},
			{From: "dup3", To: "moved3"},
		},
	}

	if got := r.duplicateFileCount(); got != 3 {
		t.Errorf("duplicateFileCount() = %d, want 3", got)
	}
	// group1: 2 duplicates * 100 bytes, group2: 1 duplicate * 50 bytes.
	if got := r.reclaimableBytes(); got != 250 {
		t.Errorf("reclaimableBytes() = %d, want 250", got)
	}
	if got := r.movedOK(); got != 2 {
		t.Errorf("movedOK() = %d, want 2", got)
	}
	if got := r.moveErrors(); got != 1 {
		t.Errorf("moveErrors() = %d, want 1", got)
	}
}

func TestWriteConsoleSummaryIncludesKeyFacts(t *testing.T) {
	r := &Report{
		SrcRoot:      `C:\src`,
		DestRoot:     `C:\dest`,
		FilesScanned: 10,
		BytesScanned: 2048,
		Groups: []DupGroup{
			{Size: 100, Files: []FileEntry{{Path: "kept"}, {Path: "dup"}}},
		},
		MoveResults: []MoveResult{{From: "dup", To: "moved"}},
	}

	var buf strings.Builder
	writeConsoleSummary(&buf, r)
	out := buf.String()

	for _, want := range []string{`C:\src`, `C:\dest`, "Duplicate groups:  1", "Files moved:       1"} {
		if !strings.Contains(out, want) {
			t.Errorf("console summary missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteTextReportListsGroupsAndWarnings(t *testing.T) {
	r := &Report{
		SrcRoot:  `C:\src`,
		DestRoot: `C:\dest`,
		Groups: []DupGroup{
			{
				Hash:  "abc123",
				Size:  10,
				Files: []FileEntry{{Path: `C:\src\a\keep.txt`}, {Path: `C:\src\b\dup.txt`}},
			},
		},
		MoveResults:  []MoveResult{{From: `C:\src\b\dup.txt`, To: `C:\dest\b\dup.txt`}},
		ScanWarnings: []string{"could not read foo.txt: permission denied"},
	}

	var buf strings.Builder
	writeTextReport(&buf, r)
	out := buf.String()

	for _, want := range []string{
		"abc123",
		`kept:   C:\src\a\keep.txt`,
		`moved:  C:\src\b\dup.txt -> C:\dest\b\dup.txt`,
		"could not read foo.txt: permission denied",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text report missing %q\ngot:\n%s", want, out)
		}
	}
}
