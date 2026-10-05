//go:build !darwin && !linux

package filemeta

import "io/fs"

func identity(info fs.FileInfo) Signature { return Signature{} }
