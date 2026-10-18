package art

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/tmdb"
)

// fakeTMDB answers the handful of endpoints the fetcher uses.
func fakeTMDB(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("/3/tv/580", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer eyJtest" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		reply(w, map[string]any{"id": 580, "poster_path": "/ds9.jpg"})
	})
	mux.HandleFunc("/3/tv/580/season/1", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"episodes": []map[string]any{
			{"episode_number": 1, "still_path": "/e1.jpg"},
			{"episode_number": 2, "still_path": ""},
		}})
	})
	mux.HandleFunc("/3/search/movie", func(w http.ResponseWriter, r *http.Request) {
		results := []map[string]any{}
		if r.URL.Query().Get("query") == "The Gold Rush" {
			results = append(results, map[string]any{"id": 962, "poster_path": "/gold.jpg"})
		}
		reply(w, map[string]any{"results": results})
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("jpeg:" + r.URL.Path)) })
	return httptest.NewServer(mux)
}

func TestSearchVariants(t *testing.T) {
	tests := map[string]string{
		"Of Mice and Men (Lon Chaney)":       "Of Mice and Men",
		"Spartacus 50th Anniversary Edition": "Spartacus",
		"Charlie Chaplin - The Gold Rush":    "The Gold Rush",
		"Lawrence Of Arabia":                 "Lawrence Of Arabia",
	}
	for in, want := range tests {
		v := searchVariants(in)
		if v[len(v)-1] != want {
			t.Errorf("searchVariants(%q) = %q, want it to end with %q", in, v, want)
		}
	}
}

func TestFetcher(t *testing.T) {
	srv := fakeTMDB(t)
	defer srv.Close()

	media := t.TempDir()
	for _, rel := range []string{
		"DS9/Star.Trek.DS9-s01e01.Emissary.mkv",
		"DS9/Star.Trek.DS9-s01e02.Past.Prologue.mkv",
		"Charlie Chaplin - The Gold Rush (1925)/gold.mkv",
		"Nothing Matches (2001)/x.mkv",
	} {
		p := filepath.Join(media, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	lib := library.Scan([]config.Library{{Path: media, Kind: "auto"}}, nil, nil)

	client := tmdb.New("eyJtest")
	client.BaseURL, client.ImgURL = srv.URL+"/3", srv.URL+"/img"
	store := NewStore(t.TempDir())
	f := &Fetcher{Client: client, Store: store, Overrides: map[string]string{"star trek ds9": "tv:580"}}
	f.Run(context.Background(), lib)

	ds9 := lib.ShowByID(library.ID("show|Star Trek DS9"))
	if !store.Has(ds9.ID) || !store.Has(ds9.Episodes[0].ID) {
		t.Errorf("missing DS9 poster or e01 still")
	}
	if store.Has(ds9.Episodes[1].ID) || !store.recentlyMissed(ds9.Episodes[1].ID) {
		t.Errorf("e02 has no still on TMDB and should be recorded as a miss")
	}
	gold := library.ID("movie|Charlie Chaplin - The Gold Rush|1925")
	if !store.Has(gold) {
		t.Errorf("Gold Rush poster not fetched via the after-the-dash retry")
	}
	if p := f.Progress(); p.Running || p.Done != p.Total || p.Total != 3 {
		t.Errorf("progress = %+v", p)
	}
	if store.Path("../../etc/passwd") != "" {
		t.Errorf("Path accepted a non-ID")
	}
}
