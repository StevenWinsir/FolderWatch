package filemeta

import "syscall"

func reliableFilesystem(root string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return false
	}
	var name []byte
	for _, b := range stat.Fstypename {
		if b == 0 {
			break
		}
		name = append(name, byte(b))
	}
	// HFS timestamps have coarser granularity; be conservative outside APFS.
	return string(name) == "apfs"
}
