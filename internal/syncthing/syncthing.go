// Package syncthing talks to the local Syncthing instance's REST API to find
// out which shared files are still on their way.
package syncthing

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Client struct {
	URL    string
	APIKey string
	HTTP   *http.Client
}

// New returns a client, discovering the API key from Syncthing itself when
// apiKey is empty.
func New(baseURL, apiKey string) *Client {
	if apiKey == "" {
		apiKey = discoverAPIKey()
	}
	return &Client{URL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

type Folder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
	Type  string `json:"type"` // sendreceive, sendonly, receiveonly, ...
}

type File struct {
	Name string `json:"name"` // relative to the folder root
	Size int64  `json:"size"`
}

// Need is what this device still has to download for a folder.
type Need struct {
	Progress []File `json:"progress"` // downloading now
	Queued   []File `json:"queued"`
	Rest     []File `json:"rest"`
}

type FolderStatus struct {
	State       string `json:"state"`
	GlobalBytes int64  `json:"globalBytes"`
	NeedBytes   int64  `json:"needBytes"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if c.APIKey == "" {
		return errors.New("no Syncthing API key")
	}
	u := c.URL + path
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("syncthing %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *Client) Folders(ctx context.Context) ([]Folder, error) {
	var fs []Folder
	return fs, c.get(ctx, "/rest/config/folders", nil, &fs)
}

func (c *Client) Need(ctx context.Context, folder string) (*Need, error) {
	n := &Need{}
	err := c.get(ctx, "/rest/db/need", url.Values{"folder": {folder}, "perpage": {"1000000"}}, n)
	for _, list := range [][]File{n.Progress, n.Queued, n.Rest} {
		for i := range list {
			list[i].Name = filepath.ToSlash(list[i].Name)
		}
	}
	return n, err
}

func (c *Client) Status(ctx context.Context, folder string) (*FolderStatus, error) {
	s := &FolderStatus{}
	return s, c.get(ctx, "/rest/db/status", url.Values{"folder": {folder}}, s)
}

// FolderFor returns the Syncthing folder whose root is dir, if any.
func FolderFor(folders []Folder, dir string) (Folder, bool) {
	for _, f := range folders {
		if samePath(f.Path, dir) {
			return f, true
		}
	}
	return Folder{}, false
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(expandHome(a)), filepath.Clean(expandHome(b))
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// discoverAPIKey asks the syncthing binary, then falls back to reading its
// config.xml from the usual places.
func discoverAPIKey() string {
	for _, exe := range syncthingBinaries() {
		out, err := exec.Command(exe, "cli", "config", "gui", "apikey", "get").Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return strings.TrimSpace(string(out))
		}
	}
	for _, p := range configFiles() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg struct {
			GUI struct {
				APIKey string `xml:"apikey"`
			} `xml:"gui"`
		}
		if xml.Unmarshal(b, &cfg) == nil && cfg.GUI.APIKey != "" {
			return cfg.GUI.APIKey
		}
	}
	return ""
}

func syncthingBinaries() []string {
	var out []string
	if runtime.GOOS == "windows" {
		out = append(out, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Syncthing", "syncthing.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Syncthing", "syncthing.exe"))
	}
	if p, err := exec.LookPath("syncthing"); err == nil {
		out = append(out, p)
	}
	var existing []string
	for _, p := range out {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	return existing
}

func configFiles() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Syncthing", "config.xml")}
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Syncthing", "config.xml")}
	default:
		return []string{
			filepath.Join(home, ".local", "state", "syncthing", "config.xml"),
			filepath.Join(home, ".config", "syncthing", "config.xml"),
		}
	}
}
