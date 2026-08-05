//go:build unix

package localfile

import (
	"os"
	"syscall"
)

// openNoFollow opens read-only and rejects a symlink at the final path component via
// O_NOFOLLOW (unix). O_NONBLOCK ensures opening a FIFO does not block waiting for a writer —
// the fstat regular-file check in Open then rejects it. O_NONBLOCK is a no-op on regular
// files, so normal reads/streams are unaffected.
func openNoFollow(path string) (*os.File, error) {
	// #nosec G304 -- path is a local file explicitly selected by the CLI user; O_NOFOLLOW +
	// the fstat regular-file check in Open reject symlinks and special files.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
