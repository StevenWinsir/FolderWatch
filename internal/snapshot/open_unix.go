//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package snapshot

import (
	"os"
	"syscall"
)

// O_NONBLOCK prevents a raced regular-file->FIFO replacement hanging capture;
// O_NOFOLLOW rejects final symlinks. Ancestors are checked separately.
func openRegular(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open snapshot", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}
