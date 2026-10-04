package watcher

import (
	"io/fs"
	"syscall"
)

func fileChangeTime(info fs.FileInfo) (int64, int64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ctimespec.Sec, st.Ctimespec.Nsec
	}
	return 0, 0
}
