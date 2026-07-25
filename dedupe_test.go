package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFindDuplicatesGroupsIdenticalContent(t *testing.T) {
	root := t.TempDir()

	// Duplicate set 1: three identical files.
	writeFile(t, filepath.Join(root, "a", "one.txt"), "same content")
	writeFile(t, filepath.Join(root, "b", "one_copy.txt"), "same content")
	writeFile(t, filepath.Join(root, "c", "one_copy2.txt"), "same content")

	// Duplicate set 2: two identical files, different content from set 1.
	writeFile(t, filepath.Join(root, "a", "two.txt"), "other content")
	writeFile(t, filepath.Join(root, "b", "two_copy.txt"), "other content")

	// Same size as set 1, but different content: must NOT be grouped with it.
	writeFile(t, filepath.Join(root, "c", "collider.txt"), "same conteNt") // same length, differs

	// Unique file with a size no one else shares.
	writeFile(t, filepath.Join(root, "a", "unique.txt"), "nobody else has this exact size!")

	entries, _, warnings := scanFiles(context.Background(), root, "", 0, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected scan warnings: %v", warnings)
	}

	groups, hashWarnings := findDuplicates(context.Background(), entries, 4, nil)
	if len(hashWarnings) != 0 {
		t.Fatalf("unexpected hash warnings: %v", hashWarnings)
	}

	if len(groups) != 2 {
		t.Fatalf("got %d duplicate groups, want 2", len(groups))
	}

	// Groups are sorted by size descending; "same content"/"other content"
	// are both 13 bytes, "same conteNt" collider is also 12/13 -- so assert
	// by looking up groups by their kept-file size instead of index.
	bySize := map[int64]DupGroup{}
	for _, g := range groups {
		bySize[g.Size] = g
	}

	g1, ok := bySize[int64(len("same content"))]
	if !ok {
		t.Fatalf("no group found for the 'same content' duplicate set")
	}
	if len(g1.Files) != 3 {
		t.Errorf("'same content' group has %d files, want 3", len(g1.Files))
	}

	g2, ok := bySize[int64(len("other content"))]
	if !ok {
		t.Fatalf("no group found for the 'other content' duplicate set")
	}
	if len(g2.Files) != 2 {
		t.Errorf("'other content' group has %d files, want 2", len(g2.Files))
	}

	// Kept file (Files[0]) must be the alphabetically first path.
	if g1.Files[0].Path != filepath.Join(root, "a", "one.txt") {
		t.Errorf("kept file = %s, want the file under a/", g1.Files[0].Path)
	}
}

func TestFindDuplicatesIgnoresEmptyFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "empty1.txt"), "")
	writeFile(t, filepath.Join(root, "b", "empty2.txt"), "")

	entries, _, _ := scanFiles(context.Background(), root, "", 0, nil)
	groups, _ := findDuplicates(context.Background(), entries, 4, nil)

	if len(groups) != 0 {
		t.Errorf("got %d groups, want 0 (empty files should be ignored)", len(groups))
	}
}

func TestFindDuplicatesNoDuplicates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "aaa")
	writeFile(t, filepath.Join(root, "b.txt"), "bbb")

	entries, _, _ := scanFiles(context.Background(), root, "", 0, nil)
	groups, _ := findDuplicates(context.Background(), entries, 4, nil)

	if len(groups) != 0 {
		t.Errorf("got %d groups, want 0", len(groups))
	}
}

func TestHashConcurrentlyStopsDispatchingOnCanceledContext(t *testing.T) {
	root := t.TempDir()

	var entries []FileEntry
	for i := 0; i < 20; i++ {
		p := filepath.Join(root, fmt.Sprintf("f%02d.txt", i))
		writeFile(t, p, "same size content!!!")
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, FileEntry{Path: p, Size: info.Size()})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before hashing starts

	hashed, warnings := hashConcurrently(ctx, entries, 2, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(hashed) >= len(entries) {
		t.Errorf("expected cancellation to stop dispatch before all %d files were hashed, got %d", len(entries), len(hashed))
	}
}
