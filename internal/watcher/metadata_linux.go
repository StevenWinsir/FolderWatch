package watcher

import (
	"io/fs"
	"syscall"
)

func fileChangeTime(info fs.FileInfo) (int64, int64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Ctim.Sec), int64(st.Ctim.Nsec)
	}
	return 0, 0
}
