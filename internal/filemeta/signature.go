// Package filemeta provides metadata hints for avoiding redundant full reads.
// Native file invalidations always force a read. Unsupported platforms return
// Strong=false and never use metadata equality to skip content verification.
package filemeta

import "io/fs"

type Signature struct {
	Device, Inode             uint64
	Size                      int64
	Mode                      uint32
	ModifiedSec, ModifiedNsec int64
	ChangedSec, ChangedNsec   int64
	Strong                    bool
}

func Read(info fs.FileInfo) Signature {
	s := identity(info)
	s.Size, s.Mode = info.Size(), uint32(info.Mode())
	s.ModifiedSec, s.ModifiedNsec = info.ModTime().Unix(), int64(info.ModTime().Nanosecond())
	return s
}

func (s Signature) Same(other Signature) bool { return s.Strong && other.Strong && s == other }
