// Package server serves the web UI and its small JSON API, and keeps the
// library, download status and artwork fresh in the background.
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/offensivegeneric/movie-night/internal/art"
	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/invite"
	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/player"
	"github.com/offensivegeneric/movie-night/internal/state"
	"github.com/offensivegeneric/movie-night/internal/syncplay"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
	"github.com/offensivegeneric/movie-night/web"
)

// Role decides who sends invites: the host owns the library and Syncthing
// sends from it; the guest receives. Solo means no shared folder was found.
type Role string

const (
	Host  Role = "host"
	Guest Role = "guest"
	Solo  Role = "solo"
)

// DetectShare finds the first library backed by a Syncthing folder. That
// folder's root holds the shared .movienight directory (invites, artwork),
// and its type decides the role: receive-only means this side is the guest.
func DetectShare(libs []config.Library, folders []syncthing.Folder) (sharedDir string, role Role) {
	for _, l := range libs {
		m, ok := syncthing.Locate(folders, l.Path)
		if !ok {
			continue
		}
		if m.Folder.Type == "receiveonly" {
			return filepath.Join(m.Folder.Path, ".movienight"), Guest
		}
		return filepath.Join(m.Folder.Path, ".movienight"), Host
	}
	return "", Solo
}

type Options struct {
	Config     *config.Config
	Syncthing  *syncthing.Client // nil if Syncthing isn't reachable
	Folders    []syncthing.Folder
	Role       Role
	InvitePath string       // "" when there's no shared folder
	ArtStores  []*art.Store // searched in order when serving images
	Fetcher    *art.Fetcher // nil without a TMDB key
	State      *state.State
}

type Download struct {
	Library     string `json:"library"`
	NeedBytes   int64  `json:"needBytes"`
	GlobalBytes int64  `json:"globalBytes"`
}

type launcher interface{ Play(path string) error }

type Server struct {
	opt      Options
	launcher launcher

	mu        sync.RWMutex
	lib       *library.Library
	downloads []Download
}

func New(opt Options) *Server {
	return &Server{opt: opt, launcher: player.Launcher{Syncplay: opt.Config.Syncplay}}
}

// Library returns the current catalogue.
func (s *Server) Library() *library.Library {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lib
}

// Refresh rescans the library, folding in what Syncthing is still
// downloading, then fetches any missing art in the background.
func (s *Server) Refresh(ctx context.Context) {
	pending := map[string][]library.Pending{}
	var downloads []Download
	if st := s.opt.Syncthing; st != nil {
		for _, l := range s.opt.Config.Libraries {
			m, ok := syncthing.Locate(s.opt.Folders, l.Path)
			if !ok {
				continue
			}
			if need, err := st.Need(ctx, m.Folder.ID); err == nil {
				for _, list := range []struct {
					files  []syncthing.File
					status library.Status
				}{{need.Progress, library.Downloading}, {need.Queued, library.Queued}, {need.Rest, library.Queued}} {
					for _, file := range list.files {
						if rel, ok := m.LibraryRel(file.Name); ok {
							pending[l.Path] = append(pending[l.Path], library.Pending{Rel: rel, Size: file.Size, Status: list.status})
						}
					}
				}
			}
			if status, err := st.Status(ctx, m.Folder.ID); err == nil && status.NeedBytes > 0 {
				downloads = append(downloads, Download{l.Name, status.NeedBytes, status.GlobalBytes})
			}
		}
	}
	lib := library.Scan(s.opt.Config.Libraries, s.opt.Config.Exclude, pending)

	s.mu.Lock()
	s.lib, s.downloads = lib, downloads
	s.mu.Unlock()

	if s.opt.Fetcher != nil {
		go s.opt.Fetcher.Run(context.WithoutCancel(ctx), lib)
	}
}

// Run refreshes every minute until ctx ends.
func (s *Server) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Refresh(ctx)
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /art/{id}", s.handleArt)
	mux.HandleFunc("GET /api/library", s.handleLibrary)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/invite", s.handleInvite)
	post := func(pattern string, h http.HandlerFunc) { mux.Handle("POST "+pattern, requireAppHeader(h)) }
	post("/api/rescan", s.handleRescan)
	post("/api/play", s.handlePlay)
	post("/api/restart", s.handleRestart)
	post("/api/invite/dismiss", s.handleDismiss)
	post("/api/syncplay/fix", s.handleFixSyncplay)
	return mux
}

// requireAppHeader rejects state-changing requests that don't carry a custom
// header. Browsers won't let another website add one to a cross-origin
// request without a CORS preflight, which this server never approves, so a
// page on some other site can't make this machine start playing things.
func requireAppHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Movie-Night") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleArt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, store := range s.opt.ArtStores {
		if store.Has(id) {
			// Revalidate (cheap: ServeFile answers 304 from the file's mtime)
			// so a corrected poster shows up straight away.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, store.Path(id))
			return
		}
	}
	http.NotFound(w, r)
}

type continueItem struct {
	Show    *library.Show    `json:"show"`
	Episode *library.Episode `json:"episode"`
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	lib := s.Library()
	var cont []continueItem
	for _, watched := range s.opt.State.Recent() {
		item, ok := lib.Lookup(watched.Episode)
		if !ok || item.Episode == nil {
			continue
		}
		if next := lib.Next(item.Episode); next != nil {
			cont = append(cont, continueItem{lib.ShowByID(next.Show), next})
		}
		if len(cont) == 6 {
			break
		}
	}
	writeJSON(w, map[string]any{"shows": lib.Shows, "movies": lib.Movies, "continue": cont})
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	s.Refresh(r.Context())
	s.handleLibrary(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	sp, _ := syncplay.Read(syncplay.ConfigPath())
	s.mu.RLock()
	downloads := s.downloads
	s.mu.RUnlock()
	status := map[string]any{
		"name":             s.opt.Config.Name,
		"role":             s.opt.Role,
		"room":             sp.Room,
		"syncplayProblems": sp.Problems(),
		"syncthing":        s.opt.Syncthing != nil,
		"downloads":        downloads,
		"tmdb":             s.opt.Fetcher != nil,
	}
	if s.opt.Fetcher != nil {
		status["art"] = s.opt.Fetcher.Progress()
	}
	writeJSON(w, status)
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.play(w, req.ID, true)
}

// handleRestart reopens the last thing played, which rejoins the room at
// its current position. It's the "something's stuck" button.
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	last := s.opt.State.Last()
	if last == nil {
		http.Error(w, "nothing has been played yet", http.StatusNotFound)
		return
	}
	s.play(w, last.Episode, false)
}

func (s *Server) play(w http.ResponseWriter, id string, sendInvite bool) {
	// Only IDs from the scan are accepted, never raw paths, so the API can
	// open library files and nothing else.
	item, ok := s.Library().Lookup(id)
	if !ok {
		http.Error(w, "unknown item", http.StatusNotFound)
		return
	}
	if !item.Ready {
		http.Error(w, item.Title+" hasn't finished downloading yet", http.StatusConflict)
		return
	}
	if err := s.launcher.Play(item.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := item.Title
	show := ""
	if item.Episode != nil {
		show = item.Episode.Show
		if item.Episode.Title != "" {
			title += " · " + item.Episode.Title
		}
	}
	s.opt.State.Played(show, id)
	log.Printf("playing %s", title)

	if sendInvite && s.opt.Role == Host && s.opt.InvitePath != "" {
		inv := invite.Invite{ID: id, Title: title, From: s.opt.Config.Name, At: time.Now()}
		if err := invite.Write(s.opt.InvitePath, inv); err != nil {
			log.Printf("writing invite: %v", err)
		}
	}
	writeJSON(w, map[string]string{"playing": title})
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	if s.opt.Role != Guest || s.opt.InvitePath == "" {
		writeJSON(w, map[string]any{})
		return
	}
	inv, err := invite.Read(s.opt.InvitePath)
	if err != nil || inv == nil || !inv.Fresh() || s.opt.State.InviteDismissed(inv.At) {
		writeJSON(w, map[string]any{})
		return
	}
	item, _ := s.Library().Lookup(inv.ID)
	writeJSON(w, map[string]any{"invite": inv, "ready": item.Ready})
}

func (s *Server) handleDismiss(w http.ResponseWriter, r *http.Request) {
	var req struct {
		At time.Time `json:"at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.opt.State.DismissInvite(req.At)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleFixSyncplay(w http.ResponseWriter, r *http.Request) {
	if err := syncplay.Fix(syncplay.ConfigPath()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
