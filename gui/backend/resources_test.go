package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/testutil"
)

func TestFacadeLifecycleReleasesDescriptorsAndDiskSnapshots(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "disk-backed.txt"), strings.Repeat("baseline line\n", 6000))
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, cache)
	}
	beforeFD, beforeG := testutil.OpenDescriptors(), runtime.NumGoroutine()
	f, api, client := testFacade(t)
	for cycle := 0; cycle < 12; cycle++ {
		req := started(t, api, client, root)
		entries, err := os.ReadDir(cache)
		if err != nil || len(entries) == 0 {
			t.Fatalf("fixture did not exercise disk snapshots: %v %v", entries, err)
		}
		if cycle%2 == 0 {
			if r := api.StopSession(req); r.Error != nil {
				t.Fatal(r)
			}
		} else {
			r := api.AttachFrontend()
			if r.Error != nil {
				t.Fatal(r)
			}
			client = r.ClientID
		}
		entries, err = os.ReadDir(cache)
		if err != nil || len(entries) != 0 {
			t.Fatalf("snapshot cache leaked on cycle %d: %v %v", cycle, entries, err)
		}
	}
	f.Close()
	eventually(t, func() bool { return runtime.NumGoroutine() <= beforeG+2 })
	afterFD := testutil.OpenDescriptors()
	if beforeFD < 0 || afterFD < 0 {
		t.Log("Descriptor measurement unavailable; cache and goroutine assertions still executed")
	} else if afterFD > beforeFD+2 {
		t.Fatalf("descriptor leak: %d -> %d", beforeFD, afterFD)
	}
	t.Logf("12 stop/reload cycles; descriptors %d -> %d; goroutines %d -> %d; cache empty", beforeFD, afterFD, beforeG, runtime.NumGoroutine())
}
