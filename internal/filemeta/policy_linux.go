package filemeta

import "syscall"

func reliableFilesystem(root string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return false
	}
	// ext-family, XFS, Btrfs and tmpfs. Everything else keeps full verification.
	switch uint64(stat.Type) {
	case 0xEF53, 0x58465342, 0x9123683E, 0x01021994:
		return true
	default:
		return false
	}
}
