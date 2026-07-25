# mvdup

A small command-line tool that finds duplicate files under a directory and
moves the extra copies out to a separate location, leaving one copy of each
file in place. It prints a console summary and writes a detailed text report.

## How it works

1. Recursively scans `-src`, recording every regular file's size.
2. Only files that share a size with at least one other file are hashed
   (SHA-256), using a pool of concurrent workers.
3. Files with identical hashes are grouped together. Within each group, the
   first file (alphabetically by path) is kept in place; the rest are
   duplicates.
4. Each duplicate is moved into `-dest`, mirroring its path relative to
   `-src`. If a file already exists at that destination path, a `_dup1`,
   `_dup2`, ... suffix is appended instead of overwriting anything.

The destination directory (if it lives inside the source tree) is excluded
from scanning, so re-running the tool won't treat already-moved files as new
duplicates.

## Build

```
go build -o mvdup.exe .
```

## Usage

```
mvdup -src <dir> [-dest <dir>] [-report <file>] [-dry-run] [-min-size <bytes>] [-workers <n>]
```

| Flag        | Default             | Description                                              |
|-------------|---------------------|-----------------------------------------------------------|
| `-src`      | *(required)*        | Directory to scan for duplicate files                     |
| `-dest`     | `./duplicates`       | Directory duplicates are moved into                        |
| `-report`   | `mvdup_report.txt`   | Path to write the detailed text report                     |
| `-dry-run`  | `false`              | Only scan and report; don't move any files                 |
| `-min-size` | `1`                  | Ignore files smaller than this many bytes                  |
| `-workers`  | number of CPUs       | Concurrent hashing workers                                  |
| `-progress` | `false`              | Print live progress to stdout while scanning, hashing, and moving |

### Example

```
mvdup -src "D:\Photos" -dest "D:\Photos\_duplicates" -report duplicates.txt
```

Do a dry run first to see what would happen without moving anything:

```
mvdup -src "D:\Photos" -dry-run
```

## Output

The console prints a short summary (files scanned, duplicate groups found,
reclaimable space, files moved). The text report additionally lists every
duplicate group with its kept file, each moved file's source/destination
path, and any warnings encountered while scanning or hashing (e.g. unreadable
files).

## License

MIT — see [LICENSE](LICENSE).
