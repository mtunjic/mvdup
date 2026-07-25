package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// hashBufSize is larger than io.Copy's default 32KB buffer, which cuts the
// number of read syscalls substantially when hashing large files.
const hashBufSize = 1 << 20 // 1 MiB

var hashBufPool = sync.Pool{
	New: func() any { return make([]byte, hashBufSize) },
}

// FileEntry describes a single regular file found during the scan.
type FileEntry struct {
	Path string // absolute path
	Size int64
	Hash string // populated later, only for hash candidates
}

// scanFiles walks root recursively and collects all regular files, returning
// the entries alongside their total size in bytes. The directory at skipAbs
// (if non-empty and rooted under root) is skipped entirely so a destination
// folder nested inside the source isn't scanned. prog may be nil; when
// non-nil it receives a live tick per file found. If ctx is canceled the walk
// stops as soon as possible, returning whatever was found so far.
func scanFiles(ctx context.Context, root, skipAbs string, minSize int64, prog *Progress) ([]FileEntry, int64, []string) {
	var entries []FileEntry
	var warnings []string
	var bytesFound int64

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
			return nil
		}
		if d.IsDir() {
			if skipAbs != "" && samePathOrChild(path, skipAbs) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, devices, etc.
		}
		info, err := d.Info()
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
			return nil
		}
		if info.Size() < minSize {
			return nil
		}
		entries = append(entries, FileEntry{Path: path, Size: info.Size()})
		bytesFound += info.Size()
		prog.ScanTick(len(entries), bytesFound)
		return nil
	})
	prog.ScanDone()

	return entries, bytesFound, warnings
}

func samePathOrChild(path, ancestor string) bool {
	rel, err := filepath.Rel(ancestor, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// hashFile returns the hex-encoded sha256 digest of the file at path.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := hashBufPool.Get().([]byte)
	defer hashBufPool.Put(buf)

	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
