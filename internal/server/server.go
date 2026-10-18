// Package server serves the web UI and its small JSON API.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/player"
	"github.com/offensivegeneric/movie-night/web"
)

type Server struct {
	cfg      *config.Config
	launcher player.Launcher

	mu  sync.RWMutex
	lib *library.Library
}

func New(cfg *config.Config, lib *library.Library) *Server {
	return &Server{cfg: cfg, lib: lib, launcher: player.Launcher{Syncplay: cfg.Syncplay}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /api/library", s.handleLibrary)
	mux.Handle("POST /api/rescan", requireAppHeader(http.HandlerFunc(s.handleRescan)))
	mux.Handle("POST /api/play", requireAppHeader(http.HandlerFunc(s.handlePlay)))
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

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, s.lib)
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	lib, err := library.Scan(s.cfg.Libraries, s.cfg.Exclude)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.lib = lib
	s.mu.Unlock()
	writeJSON(w, lib)
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Only IDs from the scan are accepted, never raw paths, so the API can
	// open library files and nothing else.
	s.mu.RLock()
	path, ok := s.lib.Path(req.ID)
	title := s.lib.Title(req.ID)
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "unknown item", http.StatusNotFound)
		return
	}
	if err := s.launcher.Play(path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("playing %s", title)
	writeJSON(w, map[string]string{"playing": title})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
