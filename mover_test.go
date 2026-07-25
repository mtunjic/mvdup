package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveDuplicatesMovesAllButKept(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dest := filepath.Join(root, "dest")

	kept := filepath.Join(src, "a", "one.txt")
	dup1 := filepath.Join(src, "b", "one_copy.txt")
	writeFile(t, kept, "dup content")
	writeFile(t, dup1, "dup content")

	group := DupGroup{
		Hash: "irrelevant",
		Size: 11,
		Files: []FileEntry{
			{Path: kept, Size: 11},
			{Path: dup1, Size: 11},
		},
	}

	results := moveDuplicates(context.Background(), []DupGroup{group}, src, dest, false, nil)
	if len(results) != 1 {
		t.Fatalf("got %d move results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected move error: %v", results[0].Err)
	}

	if _, err := os.Stat(kept); err != nil {
		t.Errorf("kept file should still exist at %s: %v", kept, err)
	}
	if _, err := os.Stat(dup1); !os.IsNotExist(err) {
		t.Errorf("duplicate should have been removed from %s", dup1)
	}

	wantDest := filepath.Join(dest, "b", "one_copy.txt")
	if results[0].To != wantDest {
		t.Errorf("moved to %s, want %s", results[0].To, wantDest)
	}
	if _, err := os.Stat(wantDest); err != nil {
		t.Errorf("expected moved file at %s: %v", wantDest, err)
	}
}

func TestMoveDuplicatesDryRunDoesNotTouchFiles(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dest := filepath.Join(root, "dest")

	kept := filepath.Join(src, "one.txt")
	dup1 := filepath.Join(src, "one_copy.txt")
	writeFile(t, kept, "dup content")
	writeFile(t, dup1, "dup content")

	group := DupGroup{
		Files: []FileEntry{{Path: kept}, {Path: dup1}},
	}

	results := moveDuplicates(context.Background(), []DupGroup{group}, src, dest, true, nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}

	if _, err := os.Stat(dup1); err != nil {
		t.Errorf("dry-run must not move files, but %s is gone: %v", dup1, err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dry-run must not create the dest directory")
	}
}

func TestMoveDuplicatesStopsOnCanceledContext(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dest := filepath.Join(root, "dest")

	kept := filepath.Join(src, "one.txt")
	dup1 := filepath.Join(src, "one_copy.txt")
	writeFile(t, kept, "dup content")
	writeFile(t, dup1, "dup content")

	group := DupGroup{
		Files: []FileEntry{{Path: kept}, {Path: dup1}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before any move starts

	results := moveDuplicates(ctx, []DupGroup{group}, src, dest, false, nil)
	if len(results) != 0 {
		t.Fatalf("got %d move results, want 0 (no move should start once canceled)", len(results))
	}
	if _, err := os.Stat(dup1); err != nil {
		t.Errorf("canceled run must not move files, but %s is gone: %v", dup1, err)
	}
}

func TestUniqueDestPathAppendsSuffixOnCollision(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dest := filepath.Join(root, "dest")

	// Pre-existing file already occupying the natural destination path.
	writeFile(t, filepath.Join(dest, "note.txt"), "already here")

	used := map[string]bool{}
	got, err := uniqueDestPath(src, dest, filepath.Join(src, "note.txt"), used)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dest, "note_dup1.txt")
	if got != want {
		t.Errorf("uniqueDestPath = %s, want %s", got, want)
	}
}

func TestUniqueDestPathAvoidsReusingPathsInSameRun(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dest := filepath.Join(root, "dest")

	used := map[string]bool{
		filepath.Join(dest, "note.txt"):      true,
		filepath.Join(dest, "note_dup1.txt"): true,
	}

	got, err := uniqueDestPath(src, dest, filepath.Join(src, "note.txt"), used)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dest, "note_dup2.txt")
	if got != want {
		t.Errorf("uniqueDestPath = %s, want %s", got, want)
	}
}

func TestMoveFileCopiesAcrossAndRemovesSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dest := filepath.Join(root, "nested", "dest.txt")
	writeFile(t, src, "payload")

	if err := moveFile(src, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source should be gone after move")
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Errorf("dest content = %q, want %q", data, "payload")
	}
}
