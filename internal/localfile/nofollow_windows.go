//go:build windows

package localfile

import (
	"os"

	"golang.org/x/sys/windows"
)

// openNoFollow opens a file on Windows WITHOUT following a symlink/reparse point at the final
// path component (NORMATIK-21). FILE_FLAG_OPEN_REPARSE_POINT opens the reparse point itself
// instead of resolving it to its target, so a symlink is never followed to another file; the
// fstat regular-file check in Open then rejects it. FILE_FLAG_BACKUP_SEMANTICS is required so
// a directory (reparse point) can also be opened-not-followed and subsequently rejected.
func openNoFollow(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- path is a local file explicitly selected by the CLI user; OPEN_REPARSE_POINT
	// refuses to follow a symlink and the fstat regular-file check in Open rejects special files.
	handle, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
