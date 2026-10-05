package pathutil

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// EvalSymlinks does not canonicalize case or Unicode-equivalent directory
// spellings on macOS. FSEvents reports filesystem spellings, so using an input
// alias as the lexical root would silently reject valid descendant events.
// F_GETPATH resolves the explicit root only; descendant keys remain untouched.
// O_EVTONLY avoids requiring directory read access merely to normalize a path,
// and Go's OpenFile supplies close-on-exec. No descriptor is retained.
func canonicalRoot(root string, expected fs.FileInfo) (string, error) {
	file, err := os.OpenFile(root, unix.O_EVTONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !opened.IsDir() || !os.SameFile(expected, opened) {
		return "", fmt.Errorf("directory changed while normalizing root")
	}
	var buffer [unix.PathMax]byte
	_, _, errno := unix.Syscall(unix.SYS_FCNTL, file.Fd(), unix.F_GETPATH, uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(file)
	if errno != 0 {
		return "", fmt.Errorf("canonical directory path: %w", errno)
	}
	end := bytes.IndexByte(buffer[:], 0)
	if end <= 0 {
		return "", fmt.Errorf("canonical directory path is empty or unterminated")
	}
	canonical := string(buffer[:end])
	if !filepath.IsAbs(canonical) {
		return "", fmt.Errorf("canonical directory path is not absolute")
	}
	// Do not adopt a replacement inode or follow a final symlink introduced
	// between EvalSymlinks, opening the root and resolving its filesystem name.
	for _, path := range []string{root, canonical} {
		current, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !current.IsDir() || !os.SameFile(opened, current) {
			return "", fmt.Errorf("directory changed while normalizing root")
		}
	}
	return canonical, nil
}
