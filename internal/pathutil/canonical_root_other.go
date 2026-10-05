//go:build !darwin

package pathutil

import "io/fs"

func canonicalRoot(root string, _ fs.FileInfo) (string, error) { return root, nil }
