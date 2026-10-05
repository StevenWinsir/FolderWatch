package filemeta

import (
	"io/fs"
	"syscall"
)

func identity(info fs.FileInfo) Signature {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return Signature{Device: uint64(st.Dev), Inode: uint64(st.Ino), ChangedSec: st.Ctimespec.Sec, ChangedNsec: st.Ctimespec.Nsec, Strong: true}
	}
	return Signature{}
}
