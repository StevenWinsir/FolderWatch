package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/StevenWinsir/FolderWatch/internal/fileutil"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/pelletier/go-toml/v2"
)

func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config directory: %w", err)
	}
	return filepath.Join(dir, "FolderWatch", "config.toml"), nil
}

// Load applies defaults < user < project < CLI. Every supplied layer must be
// valid even if later overridden. No configuration or state is written.
func Load(root string, cli Overlay, opts LoadOptions) (Config, error) {
	cfg := Defaults()
	cwd := opts.CWD
	var err error
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("working directory: %w", err)
		}
	}
	if root == "" {
		root = "."
	}
	cfg.Root, err = pathutil.NormalizeRoot(root, cwd)
	if err != nil {
		return Config{}, err
	}
	if !opts.SkipUserConfig {
		userPath := opts.UserConfigPath
		if userPath == "" {
			userPath, err = UserConfigPath()
			if err != nil {
				return Config{}, err
			}
		}
		userPath, err = pathutil.Resolve(userPath, cwd)
		if err != nil {
			return Config{}, err
		}
		if err := loadFile(&cfg, userPath, true); err != nil {
			return Config{}, err
		}
	}
	if err := loadFile(&cfg, filepath.Join(cfg.Root, ProjectFile), false); err != nil {
		return Config{}, err
	}
	if err := apply(&cfg, cli, cwd); err != nil {
		return Config{}, fmt.Errorf("CLI config: %w", err)
	}
	return cfg, cfg.Validate()
}

func loadFile(cfg *Config, path string, allowSymlink bool) error {
	data, err := fileutil.ReadConfig(path, allowSymlink)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	var layer Overlay
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&layer); err != nil {
		return fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := apply(cfg, layer, filepath.Dir(path)); err != nil {
		return fmt.Errorf("config %q: %w", path, err)
	}
	return nil
}
