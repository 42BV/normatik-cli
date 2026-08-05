// Package localfile centralises safe reads of caller-selected local files for the CLI
// (NORMATIK-21). Every read refuses symlinks and special files and is size-bounded, so an
// influenced workspace cannot make the CLI upload a secret a symlink points at, block on a
// FIFO, or exhaust memory on an oversized file.
package localfile

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotRegular is returned when a path is not a regular file (symlink, FIFO, device, dir).
var ErrNotRegular = errors.New("not a regular file (symlinks and special files are refused)")

// Open opens a local file WITHOUT following a symlink at the final path component
// (NORMATIK-21, CWE-367/CWE-59) and verifies via fstat on the OPENED descriptor that it is a
// regular file. Validating the descriptor — not re-stat'ing the path — closes the
// time-of-check/time-of-use window: a path swapped to a symlink between resolve and open is
// caught here rather than silently followed. The caller must Close the returned file.
func Open(path string) (*os.File, os.FileInfo, error) {
	f, err := openNoFollow(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return f, info, nil
}

// ReadBounded reads at most maxBytes from a regular local file (no symlink follow), erroring
// if the file exceeds maxBytes. Used for the -f form/markdown paths.
func ReadBounded(path string, maxBytes int64) ([]byte, error) {
	f, info, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s is %d bytes, which exceeds the %d-byte limit", path, info.Size(), maxBytes)
	}
	return io.ReadAll(io.LimitReader(f, maxBytes))
}
