package watcher

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestNativeDisappearanceRequiresReconcileWithoutSpuriousWarning(t *testing.T) {
	w := &FSNotify{errors: make(chan error, 2)}
	w.nativeFailure(fmt.Errorf("fsnotify.dirChange: %w", fs.ErrNotExist))
	if !w.dirty {
		t.Fatal("transient disappearance was silently dropped")
	}
	if len(w.errors) != 0 {
		t.Fatal("ordinary child disappearance became a warning")
	}
	w.dirty = false
	w.nativeFailure(fmt.Errorf("fsnotify.dirChange: %w", fs.ErrPermission))
	if !w.dirty {
		t.Fatal("permission failure did not request recovery")
	}
	select {
	case err := <-w.errors:
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("wrong error %v", err)
		}
	default:
		t.Fatal("permission failure was hidden")
	}
}
