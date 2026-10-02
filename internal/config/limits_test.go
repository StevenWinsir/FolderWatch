package config

import (
	"path/filepath"
	"testing"
)

func TestResourceLimitOverlayAndValidation(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(t.TempDir(), "config.toml")
	write(t, user, "max_pending_events = 11\nsnapshot_memory_bytes = '2MiB'\n")
	write(t, filepath.Join(root, ProjectFile), "max_pending_events = 22\nmax_watch_dirs = 33\nmax_snapshot_files = 44\nsnapshot_memory_bytes = '3MiB'\nsnapshot_cache_bytes = '4MiB'\n")
	count := 55
	memory := "5MiB"
	cfg, err := Load(root, Overlay{MaxPendingEvents: &count, SnapshotMemoryBytes: &memory}, LoadOptions{UserConfigPath: user})
	if err != nil || cfg.MaxPendingEvents != 55 || cfg.MaxWatchDirs != 33 || cfg.MaxSnapshotFiles != 44 || cfg.SnapshotMemoryBytes != 5<<20 || cfg.SnapshotCacheBytes != 4<<20 {
		t.Fatalf("limits=%+v err=%v", cfg, err)
	}
	for _, value := range []int{0, -1, 1 << 21} {
		_, err := Load(root, Overlay{MaxPendingEvents: &value}, LoadOptions{SkipUserConfig: true})
		if err == nil {
			t.Fatalf("accepted capacity %d", value)
		}
	}
	for _, value := range []string{"0", "-1MiB", "9223372036854775808"} {
		if _, err := Load(root, Overlay{SnapshotCacheBytes: &value}, LoadOptions{SkipUserConfig: true}); err == nil {
			t.Fatalf("accepted cache %s", value)
		}
	}
}
