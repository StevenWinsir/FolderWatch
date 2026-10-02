package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Fixture generation is outside the timer. B/op is cumulative allocation, not RSS.
func BenchmarkBaseline(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("%d-files-1KiB", count), func(b *testing.B) {
			root := b.TempDir()
			data := make([]byte, 1024)
			for i := range data {
				data[i] = 'x'
			}
			paths := make([]string, count)
			for i := range paths {
				paths[i] = fmt.Sprintf("f-%05d", i)
				if err := os.WriteFile(filepath.Join(root, paths[i]), data, 0600); err != nil {
					b.Fatal(err)
				}
			}
			store, err := New(root, Options{MaxFileBytes: 8 << 20, MemoryFileBytes: 64 << 10, MemoryBytes: 32 << 20, DiskBytes: 256 << 20, MaxFiles: 100001})
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			b.ReportAllocs()
			b.SetBytes(int64(count * len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.Reset(context.Background(), paths); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
