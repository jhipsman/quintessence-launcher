// Package config keeps the launcher's settings (the player name, the New World
// folder, where the isolated game copy lives and an optional server address) in
// %LOCALAPPDATA%\QuintessenceLauncher\config.json (per-user, no registry, no
// admin). Unless the player picks another folder, the same directory holds the
// game copy.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the settings file inside DataDir.
const FileName = "config.json"

// Config is the content of config.json.
type Config struct {
	// GameDir is the folder the player picked: the New World install root
	// (the one with Bin64\NewWorld.exe) or its Bin64 folder. It is read-only
	// for the launcher; the game runs from an isolated copy.
	GameDir string `json:"gameDir"`
	// CopyDir is the folder of the isolated game copy (a QuintessenceClient folder
	// inside the one the player picked). Empty means the default,
	// <DataDir>\client.
	CopyDir string `json:"copyDir,omitempty"`
	// PlayerName goes into the game as the login persona.
	PlayerName string `json:"playerName,omitempty"`
	// Server is the game server as host:port. Empty means the one the build
	// was made for (-X main.gatewayHost / main.gatewayPort).
	Server string `json:"server,omitempty"`
}

// DataDir is %LOCALAPPDATA%\QuintessenceLauncher.
func DataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "AppData", "Local")
		} else {
			base = "."
		}
	}
	return filepath.Join(base, "QuintessenceLauncher")
}

// Path is the full path of config.json.
func Path() string { return filepath.Join(DataDir(), FileName) }

// Load reads config.json. A missing file is not an error: it returns the empty
// config. A file that cannot be parsed returns the empty config and an error.
func Load() (Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", FileName, err)
	}
	return c, nil
}

// Save writes config.json atomically (temporary file + rename).
func Save(c Config) error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, Path()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
