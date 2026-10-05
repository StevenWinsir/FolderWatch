package filemeta

import (
	"io/fs"
	"syscall"
)

func identity(info fs.FileInfo) Signature {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return Signature{Device: uint64(st.Dev), Inode: uint64(st.Ino), ChangedSec: int64(st.Ctim.Sec), ChangedNsec: int64(st.Ctim.Nsec), Strong: true}
	}
	return Signature{}
}
