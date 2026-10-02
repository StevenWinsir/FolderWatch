package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
)

func TestParseInterspersedAndPresence(t *testing.T) {
	r, err := Parse([]string{"目录 with spaces", "--ignore", "*.tmp", "--respect-gitignore=false", "--ignore", "cache/", "--include-git"})
	if err != nil || r.Root != "目录 with spaces" || !reflect.DeepEqual(r.Overlay.Ignore, []string{"*.tmp", "cache/"}) || r.Overlay.RespectGitIgnore == nil || *r.Overlay.RespectGitIgnore || r.Overlay.IncludeGit == nil || !*r.Overlay.IncludeGit {
		t.Fatalf("request=%+v err=%v", r, err)
	}
	r, err = Parse(nil)
	if err != nil || r.Root != "." || r.Overlay.Debounce != nil || r.Overlay.NoMouse != nil || r.Overlay.Ignore != nil {
		t.Fatalf("defaults should not override config: %+v %v", r, err)
	}
	r, err = Parse([]string{"--", "-directory"})
	if err != nil || r.Root != "-directory" {
		t.Fatal(r, err)
	}
}

func TestHelpAndVersionBypassConfig(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "bad.toml")
	if err := os.WriteFile(bad, []byte("not valid TOML ???"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, &out, &stderr, BuildInfo{Version: "test", Commit: "abc"}, config.LoadOptions{CWD: filepath.Join(root, "missing"), UserConfigPath: bad})
		if code != 0 || out.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("help/version failed %d %s", code, stderr.String())
		}
	}
	for _, flag := range []string{"--debounce", "--ignore", "--ignore-file", "--respect-gitignore", "--include-git", "--no-mouse", "--max-diff-bytes", "--editor", "--log-file", "--debug", "--version", "--help", "--scan", "--json"} {
		if !strings.Contains(HelpText, flag) {
			t.Errorf("help missing %s", flag)
		}
	}
}

func TestRunJSONAndInvalidInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := config.LoadOptions{CWD: root, SkipUserConfig: true}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--scan", "--json", "."}, &out, &stderr, BuildInfo{}, opts); code != 0 {
		t.Fatalf("scan failed %d %s", code, stderr.String())
	}
	var result scan.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.Entries) != 2 || result.Warnings == nil {
		t.Fatalf("JSON=%s err=%v", out.String(), err)
	}
	for _, args := range [][]string{
		{"--unknown"}, {".", "extra"}, {"--ignore"}, {"--debounce", "0"}, {"--debounce", "bad"},
		{"--max-diff-bytes", "0"}, {"--max-diff-bytes", "2elephants"}, {"missing"}, {"a.txt"}, {""},
		{"--ignore", "["}, {"--ignore-file", "missing"}, {"--ignore", ""},
	} {
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), args, &out, &stderr, BuildInfo{}, opts); code != 2 || stderr.Len() == 0 {
			t.Errorf("args=%q code=%d stderr=%s", args, code, stderr.String())
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestOutputErrorsAndCancellation(t *testing.T) {
	root := t.TempDir()
	opts := config.LoadOptions{CWD: root, SkipUserConfig: true}
	for _, args := range [][]string{{"--help"}, {"--version"}, {"--json"}, {"--scan"}} {
		if code := Run(context.Background(), args, brokenWriter{}, io.Discard, BuildInfo{}, opts); code != 1 {
			t.Errorf("output error %q code=%d", args, code)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := Run(ctx, nil, io.Discard, io.Discard, BuildInfo{}, opts); code != 130 {
		t.Fatal(code)
	}
}

func TestEscapedTerminalPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows disallows these path controls")
	}
	root := t.TempDir()
	name := "escape\x1b[31m\nline"
	if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	opts := config.LoadOptions{CWD: root, SkipUserConfig: true}
	if code := Run(context.Background(), nil, &out, &stderr, BuildInfo{}, opts); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if strings.ContainsRune(out.String(), '\x1b') || strings.Contains(out.String(), "\nline") {
		t.Fatalf("unescaped path: %q", out.String())
	}
	Run(context.Background(), []string{"missing\x1b[31m"}, &out, &stderr, BuildInfo{}, opts)
	if strings.ContainsRune(stderr.String(), '\x1b') {
		t.Fatalf("unescaped diagnostic: %q", stderr.String())
	}
}
