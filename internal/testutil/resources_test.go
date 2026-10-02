package testutil

import (
	"runtime"
	"testing"
)

func TestLsofCountsOnlyNumericDescriptors(t *testing.T) {
	if got := countLsof("p100\nfcwd\nftxt\nf0\nf1\nf2\nf10001\n"); got != 4 {
		t.Fatal(got)
	}
	if got := countLsof("permission denied\n"); got != -1 {
		t.Fatal(got)
	}
}

func TestDescriptorMeasurementAvailable(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("measurement supports macOS/Linux")
	}
	if got := OpenDescriptors(); got < 3 {
		t.Fatalf("descriptor measurement unavailable: %d", got)
	}
}
