// Package testutil contains developer-only measurement helpers, not application code.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// OpenDescriptors counts numeric descriptors, excluding cwd/text mappings.
// Darwin fdescfs may reject Go ReadDir; lsof is queried for this process only.
// The lsof observation pipes are present consistently in before/after samples.
// -1 means measurement unavailable, never proof that zero descriptors remain.
func OpenDescriptors() int {
	if runtime.GOOS == "linux" {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			return -1
		}
		return len(entries)
	}
	if runtime.GOOS != "darwin" {
		return -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-n", "-P", "-a", "-p", strconv.Itoa(os.Getpid()), "-F", "f").Output()
	if err != nil {
		return -1
	}
	return countLsof(string(out))
}

func countLsof(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if len(line) > 1 && line[0] == 'f' {
			if fd, err := strconv.Atoi(line[1:]); err == nil && fd >= 0 {
				count++
			}
		}
	}
	if count == 0 {
		return -1
	}
	return count
}
