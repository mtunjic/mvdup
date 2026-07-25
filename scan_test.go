package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFilesFindsRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "one.txt"), "hello")
	writeFile(t, filepath.Join(root, "b", "two.txt"), "world!")

	entries, _, warnings := scanFiles(context.Background(), root, "", 0, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	sizes := map[string]int64{}
	for _, e := range entries {
		sizes[e.Path] = e.Size
	}
	if sizes[filepath.Join(root, "a", "one.txt")] != 5 {
		t.Errorf("wrong size for one.txt")
	}
	if sizes[filepath.Join(root, "b", "two.txt")] != 6 {
		t.Errorf("wrong size for two.txt")
	}
}

func TestScanFilesSkipsDestSubtree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.txt"), "keep me")
	writeFile(t, filepath.Join(root, "dupes", "moved.txt"), "already moved")

	destAbs := filepath.Join(root, "dupes")
	entries, _, _ := scanFiles(context.Background(), root, destAbs, 0, nil)

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (dest subtree should be skipped)", len(entries))
	}
	if entries[0].Path != filepath.Join(root, "keep.txt") {
		t.Errorf("unexpected entry: %s", entries[0].Path)
	}
}

func TestScanFilesRespectsMinSize(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "small.txt"), "hi")
	writeFile(t, filepath.Join(root, "big.txt"), "much bigger content here")

	entries, _, _ := scanFiles(context.Background(), root, "", 10, nil)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Path != filepath.Join(root, "big.txt") {
		t.Errorf("min-size filter kept the wrong file: %s", entries[0].Path)
	}
}

func TestScanFilesStopsOnCanceledContext(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "hello")
	writeFile(t, filepath.Join(root, "b.txt"), "world")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the walk starts

	entries, _, _ := scanFiles(ctx, root, "", 0, nil)
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (walk should stop immediately)", len(entries))
	}
}

func TestHashFileMatchesKnownDigest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "content.txt")
	writeFile(t, path, "hello world")

	// sha256("hello world")
	const want = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	got, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("hashFile(%q) = %s, want %s", path, got, want)
	}

	// Two identical files must hash identically, and a different one must not.
	path2 := filepath.Join(root, "content2.txt")
	writeFile(t, path2, "hello world")
	got2, err := hashFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	if got != got2 {
		t.Errorf("identical content produced different hashes: %s vs %s", got, got2)
	}

	path3 := filepath.Join(root, "content3.txt")
	writeFile(t, path3, "different content")
	got3, err := hashFile(path3)
	if err != nil {
		t.Fatal(err)
	}
	if got == got3 {
		t.Errorf("different content produced the same hash")
	}
}

func TestSamePathOrChild(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "root")
	dupes := filepath.Join(root, "dupes")

	cases := []struct {
		path, ancestor string
		want           bool
	}{
		{dupes, dupes, true},
		{filepath.Join(dupes, "a", "b.txt"), dupes, true},
		{filepath.Join(root, "other"), dupes, false},
		{root, dupes, false},
	}
	for _, c := range cases {
		got := samePathOrChild(c.path, c.ancestor)
		if got != c.want {
			t.Errorf("samePathOrChild(%q, %q) = %v, want %v", c.path, c.ancestor, got, c.want)
		}
	}
}
