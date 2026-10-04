package config

import "testing"

func TestDefaultInventoryHasNoCountCapAndExplicitLimitsRemain(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root, Overlay{}, LoadOptions{SkipUserConfig: true})
	if err != nil || cfg.MaxSnapshotFiles != 0 {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	for _, value := range []int{0, 1, 100000} {
		cfg, err := Load(root, Overlay{MaxSnapshotFiles: &value}, LoadOptions{SkipUserConfig: true})
		if err != nil || cfg.MaxSnapshotFiles != value {
			t.Fatalf("explicit %d: %+v %v", value, cfg, err)
		}
	}
	negative := -1
	if _, err := Load(root, Overlay{MaxSnapshotFiles: &negative}, LoadOptions{SkipUserConfig: true}); err == nil {
		t.Fatal("accepted negative entry cap")
	}
}
