// Package config loads the per-machine settings file. It lives outside the
// repo (in the user's config directory) because it holds local paths.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Library is one media folder and what kind of media it holds.
type Library struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Kind is "tv", "movies" or "auto" (episode codes and folder shape decide).
	Kind string `json:"kind"`
}

type Syncthing struct {
	URL    string `json:"url,omitempty"`    // default http://127.0.0.1:8384
	APIKey string `json:"apiKey,omitempty"` // default: read from Syncthing itself
}

type Config struct {
	// Name is how this person appears in invites. Default: the Syncplay name.
	Name string `json:"name"`
	// Addr is where the web UI listens. Keep it on loopback: the API can
	// launch programs on this machine.
	Addr      string    `json:"addr"`
	Libraries []Library `json:"libraries"`
	// Exclude lists file or folder names to skip anywhere in a library,
	// e.g. a torrent client's install folder that lives next to the films.
	Exclude []string `json:"exclude"`
	// Syncplay is the path to the Syncplay executable. Empty means look in
	// the usual install locations.
	Syncplay  string    `json:"syncplay"`
	Syncthing Syncthing `json:"syncthing"`
	// TMDBKey is a TMDB API key (v3) or read access token (v4). Without one,
	// posters come only from the other side's shared cache. The environment
	// variable MOVIENIGHT_TMDB_KEY overrides it.
	TMDBKey string `json:"tmdbKey"`
	// ArtOverrides pins titles that search can't match to a TMDB entry,
	// e.g. {"Star Trek DS9": "tv:580"}.
	ArtOverrides map[string]string `json:"artOverrides"`
	// Role ("host" or "guest") and SharedDir (the .movienight folder both
	// sides see) are normally worked out from Syncthing; set them to
	// override that, e.g. when the folder is shared some other way.
	Role      string `json:"role,omitempty"`
	SharedDir string `json:"sharedDir,omitempty"`
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

// Load reads path. A missing file returns an error wrapping os.ErrNotExist,
// so the caller can fall back to autodetection.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return c, c.finish()
}

func (c *Config) finish() error {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8737"
	}
	if c.Syncthing.URL == "" {
		c.Syncthing.URL = "http://127.0.0.1:8384"
	}
	if k := os.Getenv("MOVIENIGHT_TMDB_KEY"); k != "" {
		c.TMDBKey = k
	}
	if len(c.Libraries) == 0 {
		return fmt.Errorf("no libraries configured")
	}
	for i, l := range c.Libraries {
		switch l.Kind {
		case "":
			c.Libraries[i].Kind = "auto"
		case "tv", "movies", "auto":
		default:
			return fmt.Errorf("library %q: kind must be \"tv\", \"movies\" or \"auto\", got %q", l.Name, l.Kind)
		}
	}
	return nil
}

// New builds a config from detected libraries and saves it to path, so the
// user has a file to edit later.
func New(path, name string, libs []Library) (*Config, error) {
	c := &Config{Name: name, Libraries: libs, Exclude: []string{}, ArtOverrides: map[string]string{}}
	if err := c.finish(); err != nil {
		return nil, err
	}
	return c, c.Save(path)
}

// Update loads the config at path (or starts an empty one), applies name
// and libraries, and saves it, keeping everything else (TMDB key,
// overrides, excludes) as it was.
func Update(path, name string, libs []Library) (*Config, error) {
	c := &Config{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	c.Name, c.Libraries = name, libs
	if c.Exclude == nil {
		c.Exclude = []string{}
	}
	if c.ArtOverrides == nil {
		c.ArtOverrides = map[string]string{}
	}
	if err := c.finish(); err != nil {
		return nil, err
	}
	return c, c.Save(path)
}

// Save writes the config to path. It's private to the user (0600) because
// it can hold a TMDB key.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out := *c
	if os.Getenv("MOVIENIGHT_TMDB_KEY") != "" {
		// Don't copy a key that came from the environment into the file.
		if b, err := os.ReadFile(path); err == nil {
			var onDisk Config
			json.Unmarshal(b, &onDisk)
			out.TMDBKey = onDisk.TMDBKey
		} else {
			out.TMDBKey = ""
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return os.WriteFile(path, b, 0o600)
}
