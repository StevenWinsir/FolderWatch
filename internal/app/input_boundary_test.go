package app

import (
	"context"
	"errors"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/config"
)

func TestPreparedRootCannotBeRetargetedWithoutMatcher(t *testing.T) {
	p, err := Prepare(context.Background(), t.TempDir(), config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	p.Config.Root = t.TempDir()
	s, err := StartSession(context.Background(), p)
	if err == nil {
		s.Close()
		t.Fatal("accepted mismatched config and matcher")
	}
	var input *InputError
	if !errors.As(err, &input) {
		t.Fatal(err)
	}
}
