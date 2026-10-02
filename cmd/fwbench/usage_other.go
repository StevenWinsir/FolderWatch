//go:build !darwin && !linux

package main

import (
	"errors"
	"time"
)

func processUsage() (time.Duration, uint64, error) {
	return 0, 0, errors.New("fwbench resource measurements require macOS or Linux")
}
