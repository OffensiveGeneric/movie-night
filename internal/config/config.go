// Package config loads the per-machine settings file. It lives outside the
// repo (in the user's config directory) because it holds local paths.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Library is one media folder and what kind of media it holds.
type Library struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"` // "tv" or "movies"
}

type Config struct {
	// Addr is where the web UI listens. Keep it on loopback: the API can
	// launch programs on this machine.
	Addr      string    `json:"addr"`
	Libraries []Library `json:"libraries"`
	// Exclude lists file or folder names to skip anywhere in a library,
	// e.g. a torrent client's install folder that lives next to the films.
	Exclude []string `json:"exclude"`
	// Syncplay is the path to the Syncplay executable. Empty means look in
	// the usual install locations.
	Syncplay string `json:"syncplay"`
}

// DefaultPath is <user config dir>/movienight/config.json,
// e.g. %APPDATA%\movienight\config.json on Windows.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "movienight", "config.json"), nil
}

func Load(path string) (*Config, error) {
	c := &Config{Addr: "127.0.0.1:8737"}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no config at %s: copy config.example.json there and edit the library paths", path)
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(c.Libraries) == 0 {
		return nil, fmt.Errorf("%s lists no libraries", path)
	}
	for _, l := range c.Libraries {
		if l.Kind != "tv" && l.Kind != "movies" {
			return nil, fmt.Errorf("library %q: kind must be \"tv\" or \"movies\", got %q", l.Name, l.Kind)
		}
	}
	return c, nil
}
