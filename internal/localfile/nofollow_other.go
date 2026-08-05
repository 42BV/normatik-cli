//go:build !unix && !windows

package localfile

import (
	"fmt"
	"os"
)

// openNoFollow: on exotic platforms without O_NOFOLLOW (unix) or reparse-point control (windows),
// reject a symlink at the final path component via lstat before opening. This leaves a narrow
// lstat->open TOCTOU window, acceptable for these rarely-targeted build targets; the fstat
// regular-file check in Open still rejects special files.
func openNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	// #nosec G304 -- path is a local file explicitly selected by the CLI user; lstat rejects a
	// symlink above and the fstat regular-file check in Open rejects special files.
	return os.OpenFile(path, os.O_RDONLY, 0)
}
