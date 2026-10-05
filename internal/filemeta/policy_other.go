//go:build !darwin && !linux

package filemeta

func reliableFilesystem(string) bool { return false }
