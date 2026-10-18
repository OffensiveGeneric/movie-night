package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/state"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
	"github.com/offensivegeneric/movie-night/internal/syncthing/stfake"
)

// fakeLauncher stands in for Syncplay.
type fakeLauncher struct{ played []string }

func (f *fakeLauncher) Play(path string) error { f.played = append(f.played, path); return nil }

func newTestServer(t *testing.T, role Role, media, invitePath string) (*Server, *fakeLauncher) {
	t.Helper()
	cfg := &config.Config{Name: string(role), Libraries: []config.Library{{Name: "TV", Path: media, Kind: "tv"}}}
	s := New(Options{Config: cfg, Role: role, InvitePath: invitePath, State: state.Load(filepath.Join(t.TempDir(), "state.json"))})
	fl := &fakeLauncher{}
	s.launcher = fl
	s.Refresh(context.Background())
	return s, fl
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("X-Movie-Night", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestInviteHostToGuest(t *testing.T) {
	ep := "Star.Trek.DS9-s01e12.Battle.Lines.mkv"
	hostMedia, guestMedia := t.TempDir(), t.TempDir()
	for _, dir := range []string{hostMedia, guestMedia} {
		os.WriteFile(filepath.Join(dir, ep), []byte("x"), 0o644)
	}
	// Both sides point at the "same" shared file; in real life Syncthing
	// copies it from the host's folder to the guest's.
	invitePath := filepath.Join(t.TempDir(), ".movienight", "invite.json")
	host, hostPlayer := newTestServer(t, Host, hostMedia, invitePath)
	guest, guestPlayer := newTestServer(t, Guest, guestMedia, invitePath)

	id := library.ID("ep|Star Trek DS9|1|12")
	if rec := call(t, host.Handler(), "POST", "/api/play", `{"id":"`+id+`"}`); rec.Code != 200 {
		t.Fatalf("host play: %d %s", rec.Code, rec.Body)
	}
	if len(hostPlayer.played) != 1 {
		t.Fatalf("host didn't launch the player")
	}

	var got struct {
		Invite *struct{ ID, Title, From string } `json:"invite"`
		Ready  bool                              `json:"ready"`
	}
	json.Unmarshal(call(t, guest.Handler(), "GET", "/api/invite", "").Body.Bytes(), &got)
	if got.Invite == nil || got.Invite.ID != id || !got.Ready || got.Invite.From != "host" {
		t.Fatalf("guest invite = %+v", got)
	}
	if !strings.Contains(got.Invite.Title, "Battle Lines") {
		t.Errorf("invite title %q should name the episode", got.Invite.Title)
	}

	// Joining plays the guest's own copy, found by the shared ID.
	if rec := call(t, guest.Handler(), "POST", "/api/play", `{"id":"`+id+`"}`); rec.Code != 200 {
		t.Fatalf("guest join: %d %s", rec.Code, rec.Body)
	}
	if len(guestPlayer.played) != 1 || !strings.HasPrefix(guestPlayer.played[0], guestMedia) {
		t.Fatalf("guest played %v, want its own file", guestPlayer.played)
	}

	// A guest's own play doesn't write invites, and the host never shows one.
	if body := call(t, host.Handler(), "GET", "/api/invite", "").Body.String(); strings.TrimSpace(body) != "{}" {
		t.Errorf("host /api/invite = %s, want {}", body)
	}

	// Continue watching now offers the next episode... except there isn't
	// one here, so the list is empty rather than pointing at nothing.
	var lib struct {
		Continue []any `json:"continue"`
	}
	json.Unmarshal(call(t, guest.Handler(), "GET", "/api/library", "").Body.Bytes(), &lib)
	if len(lib.Continue) != 0 {
		t.Errorf("continue = %v, want empty with no next episode", lib.Continue)
	}
}

func TestDetectShare(t *testing.T) {
	videos := filepath.Join(string(filepath.Separator), "Users", "Dad", "Videos")
	share := filepath.Join(videos, "Movie Night")
	folders := []syncthing.Folder{
		{ID: "movie-night", Path: share, Type: "receiveonly"},
		{ID: "movie-night-movies", Path: filepath.Join(videos, "Movie Night - Movies"), Type: "receiveonly"},
	}
	tests := []struct {
		name string
		lib  string
		dir  string
		role Role
	}{
		{"library is the share", share, filepath.Join(share, ".movienight"), Guest},
		{"library contains the share", videos, filepath.Join(share, ".movienight"), Guest},
		{"library inside the share", filepath.Join(share, "TV"), filepath.Join(share, ".movienight"), Guest},
		{"not shared", filepath.Join(string(filepath.Separator), "Other"), "", Solo},
	}
	for _, tt := range tests {
		dir, role := DetectShare([]config.Library{{Path: tt.lib}}, folders)
		if dir != tt.dir || role != tt.role {
			t.Errorf("%s: got %q %s, want %q %s", tt.name, dir, role, tt.dir, tt.role)
		}
	}
	host := []syncthing.Folder{{ID: "movie-night", Path: share, Type: "sendonly"}}
	if _, role := DetectShare([]config.Library{{Path: share}}, host); role != Host {
		t.Errorf("send-only folder should make this side the host, got %s", role)
	}
}

func TestAcceptPairing(t *testing.T) {
	fake := stfake.New("HOST-ID")
	defer fake.Close()
	tv := t.TempDir()
	fake.AddFolder("movie-night", "Movie Night", tv, "sendonly")
	fake.Pending["GUEST-ID"] = map[string]any{"name": "Dad", "address": "1.2.3.4:22000"}
	st := fake.Client()
	folders, _ := st.Folders(context.Background())

	cfg := &config.Config{Name: "Tyler", Libraries: []config.Library{{Name: "TV", Path: tv, Kind: "tv"}}}
	s := New(Options{Config: cfg, Role: Host, Syncthing: st, Folders: folders, State: state.Load(filepath.Join(t.TempDir(), "s.json"))})
	s.launcher = &fakeLauncher{}
	s.Refresh(context.Background())
	h := s.Handler()

	if body := call(t, h, "GET", "/api/pairing", "").Body.String(); !strings.Contains(body, "GUEST-ID") {
		t.Fatalf("pairing list = %s", body)
	}
	if rec := call(t, h, "POST", "/api/pairing/accept", `{"id":"SOMEONE-ELSE"}`); rec.Code != http.StatusNotFound {
		t.Errorf("accepting a device that never asked: %d, want 404", rec.Code)
	}
	if rec := call(t, h, "POST", "/api/pairing/accept", `{"id":"GUEST-ID"}`); rec.Code != 200 {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body)
	}
	if fake.Devices["GUEST-ID"] == nil || fake.Devices["GUEST-ID"]["name"] != "Dad" {
		t.Errorf("guest device not added: %+v", fake.Devices)
	}
	if d := fake.FolderDevices("movie-night"); len(d) != 1 || d[0] != "GUEST-ID" {
		t.Errorf("TV not shared with guest: %v", d)
	}
	if body := call(t, h, "GET", "/api/pairing", "").Body.String(); strings.Contains(body, "GUEST-ID") {
		t.Errorf("guest still pending after accept: %s", body)
	}
}

func TestPostNeedsAppHeader(t *testing.T) {
	s, fl := newTestServer(t, Solo, t.TempDir(), "")
	req := httptest.NewRequest("POST", "/api/play", strings.NewReader(`{"id":"x"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || len(fl.played) != 0 {
		t.Errorf("POST without header: %d, played %v", rec.Code, fl.played)
	}
}
