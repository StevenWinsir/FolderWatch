package config

import (
	"path/filepath"
	"testing"
)

func TestClassificationAndDiffLimitsAreIndependent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ProjectFile), "max_snapshot_bytes='2MiB'\nmax_diff_bytes='1MiB'\nmax_diff_lines=150\n")
	lines := 20
	size := "3MiB"
	cfg, err := Load(root, Overlay{MaxDiffLines: &lines, MaxSnapshotBytes: &size}, LoadOptions{SkipUserConfig: true})
	if err != nil || cfg.MaxDiffBytes != 1<<20 || cfg.MaxSnapshotBytes != 3<<20 || cfg.MaxDiffLines != 20 {
		t.Fatalf("%+v %v", cfg, err)
	}
	for _, bad := range []string{"0", "65MiB", "-1", "bad"} {
		if _, err := Load(root, Overlay{MaxSnapshotBytes: &bad}, LoadOptions{SkipUserConfig: true}); err == nil {
			t.Fatalf("accepted snapshot cap %q", bad)
		}
	}
	for _, bad := range []int{0, -1, 200001} {
		if _, err := Load(root, Overlay{MaxDiffLines: &bad}, LoadOptions{SkipUserConfig: true}); err == nil {
			t.Fatalf("accepted line cap %d", bad)
		}
	}
}
