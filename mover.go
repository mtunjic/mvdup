package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MoveResult records the outcome of relocating a single duplicate file.
type MoveResult struct {
	From string
	To   string
	Size int64
	Err  error
}

// moveDuplicates moves every duplicate in each group (everything but Files[0],
// which is left in place as the kept original) into destRoot, mirroring the
// file's path relative to srcRoot. In dry-run mode no files are touched and
// To is filled in with what the destination would have been. prog may be
// nil; when non-nil it receives a live line per move. If ctx is canceled,
// no further moves are started; a move already in progress always runs to
// completion first, so a file is never left half-copied.
func moveDuplicates(ctx context.Context, groups []DupGroup, srcRoot, destRoot string, dryRun bool, prog *Progress) []MoveResult {
	var results []MoveResult
	used := make(map[string]bool)

	for _, g := range groups {
		for _, dup := range g.Files[1:] {
			if ctx.Err() != nil {
				return results
			}

			dest, err := uniqueDestPath(srcRoot, destRoot, dup.Path, used)
			if err != nil {
				results = append(results, MoveResult{From: dup.Path, Size: dup.Size, Err: err})
				prog.Move(dup.Path, "", dryRun, err)
				continue
			}
			used[dest] = true

			res := MoveResult{From: dup.Path, To: dest, Size: dup.Size}
			if !dryRun {
				if err := moveFile(dup.Path, dest); err != nil {
					res.Err = err
				}
			}
			results = append(results, res)
			prog.Move(res.From, res.To, dryRun, res.Err)
		}
	}

	return results
}

// uniqueDestPath mirrors src's path relative to srcRoot under destRoot,
// appending _dup1, _dup2, ... if that path is already taken (by a previous
// run or another file in this run).
func uniqueDestPath(srcRoot, destRoot, src string, used map[string]bool) (string, error) {
	rel, err := filepath.Rel(srcRoot, src)
	if err != nil {
		return "", err
	}
	base := filepath.Join(destRoot, rel)

	if _, err := os.Stat(base); os.IsNotExist(err) && !used[base] {
		return base, nil
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_dup%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) && !used[candidate] {
			return candidate, nil
		}
	}
}

func moveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	// os.Rename fails across filesystems/volumes; fall back to copy+remove.
	return copyThenRemove(src, dest)
}

func copyThenRemove(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dest)
		return err
	}

	return os.Remove(src)
}
