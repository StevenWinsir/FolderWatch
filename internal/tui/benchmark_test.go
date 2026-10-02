package tui

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

// Models only: no watcher, disk access, renderer scheduling or terminal compositor.
func BenchmarkLargeChangeList(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			m := newModel(context.Background(), "/project", nil, io.Discard)
			m.palette.plain = true
			m.width, m.height = 120, 36
			view := changes.View{Generation: 1, Version: 1}
			for i := 0; i < count; i++ {
				view.Changes = append(view.Changes, changes.Summary{Path: fmt.Sprintf("file-%05d.txt", i), Kind: changes.Modified, Version: 1})
			}
			m.acceptView(view)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				view.Version++
				m.acceptView(view)
				_ = m.View()
			}
		})
	}
}
