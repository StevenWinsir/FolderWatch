package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func write(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSize(t *testing.T) {
	for value, want := range map[string]int64{"1": 1, "1KB": 1000, "1KiB": 1024, "1.5MiB": 1572864, " 2mb ": 2000000, "0.001KB": 1, "9223372036854775807": 9223372036854775807} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseSize(value)
			if err != nil || got != want {
				t.Fatalf("got %d %v want %d", got, err, want)
			}
		})
	}
	for _, value := range []string{"", "0", "-1", "1.1B", "0.1", "NaN", "1PB", "1e6", "9223372036854775808", "9999999999999999999999GiB", "1 M", strings.Repeat("9", 129)} {
		t.Run("invalid_"+value[:min(len(value), 20)], func(t *testing.T) {
			if _, err := ParseSize(value); err == nil {
				t.Fatalf("accepted %q", value)
			}
		})
	}
}

func TestPrecedenceAndExplicitFalse(t *testing.T) {
	root, userDir := t.TempDir(), t.TempDir()
	user := filepath.Join(userDir, "config.toml")
	write(t, user, "debounce = '300ms'\nmax_diff_bytes = '2MiB'\nignore = ['user']\nrespect_gitignore = true\nno_mouse = true\ndebug = true\n")
	write(t, filepath.Join(root, ProjectFile), "debounce = '200ms'\nignore = ['project']\ninclude_git = true\n")
	opts := LoadOptions{CWD: root, UserConfigPath: user}
	cfg, err := Load(".", Overlay{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Debounce != 200*time.Millisecond || cfg.MaxDiffBytes != 2<<20 || !cfg.RespectGitIgnore || !cfg.NoMouse || !cfg.IncludeGit || !reflect.DeepEqual(cfg.Ignore, []string{"project"}) {
		t.Fatalf("precedence: %+v", cfg)
	}
	cfg, err = Load(".", Overlay{Debounce: ptr("75ms"), Ignore: []string{}, RespectGitIgnore: ptr(false), NoMouse: ptr(false), Debug: ptr(false), IncludeGit: ptr(false)}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Debounce != 75*time.Millisecond || len(cfg.Ignore) != 0 || cfg.RespectGitIgnore || cfg.NoMouse || cfg.Debug || cfg.IncludeGit {
		t.Fatalf("explicit overrides: %+v", cfg)
	}
}

func TestDefaultsAndPaths(t *testing.T) {
	root, userDir, cwd := t.TempDir(), t.TempDir(), t.TempDir()
	user := filepath.Join(userDir, "config.toml")
	write(t, filepath.Join(userDir, "rules"), "*.tmp\n")
	write(t, user, "ignore_file = 'rules'\nlog_file = 'debug.log'\neditor = 'editor --wait'\n")
	cfg, err := Load(root, Overlay{}, LoadOptions{CWD: cwd, UserConfigPath: user})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IgnoreFile != filepath.Join(userDir, "rules") || cfg.LogFile != filepath.Join(userDir, "debug.log") || cfg.Editor != "editor --wait" {
		t.Fatalf("relative config paths: %+v", cfg)
	}
	if _, err := os.Stat(cfg.LogFile); !os.IsNotExist(err) {
		t.Fatal("config created log file")
	}
	write(t, filepath.Join(root, "project.rules"), "*.bak\n")
	write(t, filepath.Join(root, ProjectFile), "ignore_file = 'project.rules'\n")
	cfg, err = Load(root, Overlay{}, LoadOptions{CWD: cwd, UserConfigPath: user})
	if err != nil {
		t.Fatal(err)
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	if cfg.IgnoreFile != filepath.Join(realRoot, "project.rules") {
		t.Fatal(cfg.IgnoreFile)
	}
	write(t, filepath.Join(cwd, "cli.rules"), "*.cli\n")
	cfg, err = Load(root, Overlay{IgnoreFile: ptr("cli.rules")}, LoadOptions{CWD: cwd, UserConfigPath: user})
	if err != nil || cfg.IgnoreFile != filepath.Join(cwd, "cli.rules") {
		t.Fatalf("CLI paths: %+v %v", cfg, err)
	}
	plain, err := Load(cwd, Overlay{}, LoadOptions{SkipUserConfig: true})
	if err != nil || plain.Debounce != 150*time.Millisecond || plain.MaxDiffBytes != 5<<20 || plain.RespectGitIgnore {
		t.Fatalf("defaults: %+v %v", plain, err)
	}
}

func TestInvalidConfig(t *testing.T) {
	cases := []string{
		"debounce = '0ms'", "debounce = '-1s'", "debounce = 'forever'", "debounce = '9999999999999h'",
		"max_diff_bytes = '0'", "max_diff_bytes = '-5MB'", "max_diff_bytes = '12elephants'",
		"unknown_setting = true", "debug = 'yes'", "debounce = 100", "ignore = ['']", "ignore = 2",
		"ignore_file = 'missing'", "log_file = 'missing-parent/log'", "log_file = '.'", "[broken",
	}
	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, ProjectFile), text)
			_, err := Load(root, Overlay{}, LoadOptions{SkipUserConfig: true})
			if err == nil || !strings.Contains(err.Error(), "config") {
				t.Fatalf("expected actionable config error, got %v", err)
			}
		})
	}
}

func TestInvalidRootAndLowerLayerNotHidden(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "file"), "x")
	for _, p := range []string{filepath.Join(root, "missing"), filepath.Join(root, "file"), "bad\x00path"} {
		if _, err := Load(p, Overlay{}, LoadOptions{SkipUserConfig: true}); err == nil {
			t.Fatalf("accepted root %q", p)
		}
	}
	user := filepath.Join(root, "user.toml")
	write(t, user, "debounce = 'bad'\n")
	if _, err := Load(root, Overlay{Debounce: ptr("10ms")}, LoadOptions{UserConfigPath: user}); err == nil {
		t.Fatal("masked bad user config")
	}
}

func TestMalformedLowerPriorityGlobIsNotHidden(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(t.TempDir(), "config.toml")
	write(t, user, "ignore = ['[']\n")
	if _, err := Load(root, Overlay{Ignore: []string{"*.tmp"}}, LoadOptions{UserConfigPath: user}); err == nil {
		t.Fatal("CLI override masked malformed user ignore rule")
	}
}

func FuzzSize(f *testing.F) {
	for _, v := range []string{"1", "0", "1.5MiB", "9223372036854775808", "banana"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, value string) {
		n, err := ParseSize(value)
		if err == nil && n <= 0 {
			t.Fatalf("non-positive size %d", n)
		}
	})
}
