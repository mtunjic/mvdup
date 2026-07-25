package main

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
)

// DupGroup is a set of files that are byte-for-byte identical.
// Files[0] is the one that is kept in place; the rest are duplicates.
type DupGroup struct {
	Hash  string
	Size  int64
	Files []FileEntry
}

// findDuplicates groups entries by size (cheap), hashes only the files that
// share a size with at least one other file, then groups those by hash.
// Groups with a single hashed file (same size, different content) are dropped.
// prog may be nil; when non-nil it receives a live tick per file hashed. If
// ctx is canceled, hashing of not-yet-started files is skipped and grouping
// proceeds with whatever was hashed already.
func findDuplicates(ctx context.Context, entries []FileEntry, workers int, prog *Progress) (groups []DupGroup, warnings []string) {
	bySize := make(map[int64][]FileEntry)
	for _, e := range entries {
		bySize[e.Size] = append(bySize[e.Size], e)
	}

	var candidates []FileEntry
	for size, files := range bySize {
		if size == 0 {
			continue // every empty file hashes the same but carries no content to reclaim
		}
		if len(files) > 1 {
			candidates = append(candidates, files...)
		}
	}

	hashed, hashWarnings := hashConcurrently(ctx, candidates, workers, prog)
	warnings = append(warnings, hashWarnings...)

	byHash := make(map[string][]FileEntry)
	for _, e := range hashed {
		byHash[e.Hash] = append(byHash[e.Hash], e)
	}

	for hash, files := range byHash {
		if len(files) < 2 {
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		groups = append(groups, DupGroup{Hash: hash, Size: files[0].Size, Files: files})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Size != groups[j].Size {
			return groups[i].Size > groups[j].Size
		}
		return groups[i].Hash < groups[j].Hash
	})

	return groups, warnings
}

func hashConcurrently(ctx context.Context, entries []FileEntry, workers int, prog *Progress) ([]FileEntry, []string) {
	if len(entries) == 0 {
		return nil, nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(entries) {
		workers = len(entries) // no point starting more workers than there is work
	}

	jobs := make(chan int)
	results := make([]FileEntry, len(entries))
	var warnings []string
	var warnMu sync.Mutex
	var wg sync.WaitGroup
	var done atomic.Int64

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				e := entries[i]
				hash, err := hashFile(e.Path)
				if err != nil {
					warnMu.Lock()
					warnings = append(warnings, e.Path+": "+err.Error())
					warnMu.Unlock()
					prog.HashTick(int(done.Add(1)), len(entries))
					continue
				}
				e.Hash = hash
				results[i] = e

				prog.HashTick(int(done.Add(1)), len(entries))
			}
		}()
	}

dispatch:
	for i := range entries {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch // let already-dispatched hashes finish; start no new ones
		}
	}
	close(jobs)
	wg.Wait()

	final := results[:0]
	for _, e := range results {
		if e.Hash != "" {
			final = append(final, e)
		}
	}
	return final, warnings
}
