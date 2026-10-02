//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package snapshot

import "os"

// Other platforms have compile coverage only. Lstat and opened identity checks
// still apply; adversarial filesystem replacement is not a sandbox guarantee.
func openRegular(path string) (*os.File, error) { return os.Open(path) }
