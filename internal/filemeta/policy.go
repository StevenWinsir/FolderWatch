package filemeta

import (
	"io/fs"
	"os"
)

// Policy permits metadata-only hash reuse only on recognized local filesystems.
// Unknown/network filesystems and nested mount devices force content reads.
// Direct native file invalidations force reads regardless of this optimization.
type Policy struct {
	Device  uint64
	Trusted bool
}

func PolicyForRoot(root string) Policy {
	info, err := os.Lstat(root)
	if err != nil {
		return Policy{}
	}
	return Policy{Device: Read(info).Device, Trusted: reliableFilesystem(root)}
}
func (p Policy) Read(info fs.FileInfo) Signature {
	signature := Read(info)
	// ext-family magic also covers coarse-timestamp older filesystems. A zero
	// ctime fraction is not enough evidence for timestamp-based hash reuse;
	// conservatively re-read even on a trusted filesystem at that boundary.
	signature.Strong = signature.Strong && p.Trusted && signature.Device == p.Device && signature.ChangedNsec != 0
	return signature
}
