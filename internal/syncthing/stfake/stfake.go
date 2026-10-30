// Package stfake is an in-memory stand-in for Syncthing's REST API, for
// tests of code that configures Syncthing.
package stfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/offensivegeneric/movie-night/internal/syncthing"
)

type Fake struct {
	*httptest.Server
	MyID string

	mu      sync.Mutex
	Folders map[string]map[string]any
	Devices map[string]map[string]any
	Pending map[string]map[string]any
}

// New starts a fake Syncthing. Point a syncthing.Client at f.URL with any
// non-empty API key.
func New(myID string) *Fake {
	f := &Fake{MyID: myID, Folders: map[string]map[string]any{}, Devices: map[string]map[string]any{}, Pending: map[string]map[string]any{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// Client returns a client for the fake.
func (f *Fake) Client() *syncthing.Client {
	c := syncthing.New(f.URL, "test-key")
	return c
}

// AddFolder seeds an existing folder.
func (f *Fake) AddFolder(id, label, path, typ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Folders[id] = map[string]any{"id": id, "label": label, "path": path, "type": typ, "devices": []any{}}
}

// FolderDevices lists the device IDs a folder is shared with.
func (f *Fake) FolderDevices(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	devs, _ := f.Folders[id]["devices"].([]any)
	for _, d := range devs {
		if m, ok := d.(map[string]any); ok {
			s, _ := m["deviceID"].(string)
			out = append(out, s)
		}
	}
	return out
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-API-Key") == "" {
		http.Error(w, "no key", http.StatusForbidden)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(v any) { json.NewEncoder(w).Encode(v) }
	var body map[string]any
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	path := r.URL.Path

	switch {
	case path == "/rest/system/status":
		reply(map[string]any{"myID": f.MyID})
	case path == "/rest/config/defaults/folder":
		reply(map[string]any{"rescanIntervalS": 3600, "fsWatcherEnabled": true, "order": "random", "devices": []any{}})
	case path == "/rest/config/defaults/device":
		reply(map[string]any{"compression": "metadata", "addresses": []any{"dynamic"}})
	case path == "/rest/config/folders":
		var list []any
		for _, v := range f.Folders {
			list = append(list, v)
		}
		reply(list)
	case strings.HasPrefix(path, "/rest/config/folders/"):
		id := strings.TrimPrefix(path, "/rest/config/folders/")
		f.object(w, r, f.Folders, id, body)
	case strings.HasPrefix(path, "/rest/config/devices/"):
		id := strings.TrimPrefix(path, "/rest/config/devices/")
		f.object(w, r, f.Devices, id, body)
		if r.Method == "PUT" {
			delete(f.Pending, id) // like Syncthing: adding a device clears its pending entry
		}
	case path == "/rest/cluster/pending/devices":
		reply(f.Pending)
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) object(w http.ResponseWriter, r *http.Request, store map[string]map[string]any, id string, body map[string]any) {
	switch r.Method {
	case "GET":
		v, ok := store[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(v)
	case "PUT":
		store[id] = body
	case "PATCH":
		v, ok := store[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		for k, val := range body {
			v[k] = val
		}
	}
}
