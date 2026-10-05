//go:build !darwin || !cgo

package watcher

func preferredWatcher(opts Options) (Watcher, error) { return New(opts) }
