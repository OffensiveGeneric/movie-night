// Package syncthing talks to the local Syncthing instance's REST API to find
// out which shared files are still on their way.
package syncthing

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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

// StatusError is a non-200 answer from Syncthing.
type StatusError struct {
	Path string
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("syncthing %s: HTTP %d", e.Path, e.Code) }

func isNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, q, nil, out)
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	if c.APIKey == "" {
		return errors.New("no Syncthing API key")
	}
	u := c.URL + path
	if q != nil {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &StatusError{path, res.StatusCode}
	}
	if out == nil {
		return nil
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

// Match ties a library folder to the Syncthing folder that covers it. They
// can be the same folder, the library can sit inside the Syncthing folder
// (Sub is its path there), or the library can contain it (Outer is the
// Syncthing folder's path inside the library).
type Match struct {
	Folder     Folder
	Sub, Outer string // forward slashes; at most one is set
}

// Locate finds the Syncthing folder for a library directory: an exact or
// enclosing folder first, then one inside the library.
func Locate(folders []Folder, dir string) (Match, bool) {
	for _, f := range folders {
		if rel, ok := within(f.Path, dir); ok {
			return Match{Folder: f, Sub: rel}, true
		}
	}
	for _, f := range folders {
		if rel, ok := within(dir, f.Path); ok {
			return Match{Folder: f, Outer: rel}, true
		}
	}
	return Match{}, false
}

// LibraryRel converts a path relative to the Syncthing folder into one
// relative to the library, or reports false if it's outside the library.
func (m Match) LibraryRel(name string) (string, bool) {
	name = filepath.ToSlash(name)
	switch {
	case m.Sub != "":
		prefix := m.Sub + "/"
		if len(name) <= len(prefix) || !samePart(name[:len(prefix)], prefix) {
			return "", false
		}
		return name[len(prefix):], true
	case m.Outer != "":
		return m.Outer + "/" + name, true
	}
	return name, true
}

var caseInsensitive = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

func samePart(a, b string) bool {
	if caseInsensitive {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// within reports whether child is parent or below it, and child's path
// relative to parent ("" when they're the same).
func within(parent, child string) (string, bool) {
	p := strings.TrimRight(filepath.Clean(expandHome(parent)), `\/`)
	c := filepath.Clean(expandHome(child))
	if samePart(strings.TrimRight(c, `\/`), p) {
		return "", true
	}
	sep := string(filepath.Separator)
	if len(c) <= len(p)+1 || !samePart(c[:len(p)+1], p+sep) {
		return "", false
	}
	return filepath.ToSlash(c[len(p)+1:]), true
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
	for _, exe := range Binaries() {
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

// Binaries lists installed syncthing executables, most likely first.
func Binaries() []string {
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
