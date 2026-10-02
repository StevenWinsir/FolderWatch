// Package fileutil provides bounded reads for configuration, never file content.
package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const MaxConfigBytes = 1 << 20

// ReadConfig reads a regular file up to MaxConfigBytes. Automatically discovered
// config/rule files must not be symlinks; explicit user selections may be.
func ReadConfig(path string, allowSymlink bool) ([]byte, error) {
	if allowSymlink {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		path = resolved
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q must be a regular file (automatic symlinks are not followed)", path)
	}
	if info.Size() > MaxConfigBytes {
		return nil, fmt.Errorf("%q exceeds the %d-byte config/ignore limit", path, MaxConfigBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%q changed while opening configuration", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxConfigBytes {
		return nil, fmt.Errorf("%q exceeds the %d-byte config/ignore limit", path, MaxConfigBytes)
	}
	return data, nil
}
