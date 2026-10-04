//go:build !darwin && !linux

package watcher

import "io/fs"

// Other platforms retain identity/size/mode/mtime detection in fallback mode.
func fileChangeTime(info fs.FileInfo) (int64, int64) { return 0, 0 }
