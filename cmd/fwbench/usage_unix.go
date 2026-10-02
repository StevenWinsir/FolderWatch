//go:build darwin || linux

package main

import (
	"runtime"
	"syscall"
	"time"
)

func processUsage() (time.Duration, uint64, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, 0, err
	}
	cpu := time.Duration(usage.Utime.Sec+usage.Stime.Sec)*time.Second + time.Duration(usage.Utime.Usec+usage.Stime.Usec)*time.Microsecond
	rss := uint64(usage.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return cpu, rss, nil
}
