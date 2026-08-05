package cli

import (
	"io"
	"os"
	"path/filepath"
)

func atomicWrite(target string, source io.Reader) (written int64, err error) {
	dir := filepath.Dir(target)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempName)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		return 0, err
	}
	written, err = io.Copy(temp, source)
	if err != nil {
		return written, err
	}
	if err := temp.Sync(); err != nil {
		return written, err
	}
	if err := temp.Close(); err != nil {
		return written, err
	}
	if err := replaceFile(tempName, target); err != nil {
		return written, err
	}
	committed = true
	return written, nil
}
