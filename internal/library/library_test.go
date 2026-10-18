package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/offensivegeneric/movie-night/internal/config"
)

func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAutoAndPending(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "Star.Trek.DS9.S1/Star.Trek.DS9-s01e01.Emissary.mkv")
	touch(t, root, "Star.Trek.DS9.S1/Star.Trek.DS9-s01e02.Past.Prologue.mkv")
	touch(t, root, "Baseball (1994)/Inning 1.mkv")
	touch(t, root, "Baseball (1994)/Inning 2.mkv")
	touch(t, root, "Office Space (1999)/Office.Space.mkv")
	touch(t, root, "Harold.and.Maude.1971.mkv")
	touch(t, root, ".movienight/invite.json")
	touch(t, root, "Deluge/app.mkv")

	pending := map[string][]Pending{root: {
		{Rel: "Star.Trek.DS9.S1/Star.Trek.DS9-s01e03.A.Man.Alone.mkv", Status: Downloading},
		{Rel: "Star.Trek.DS9.S1/Star.Trek.DS9-s01e02.Past.Prologue.mkv", Status: Queued}, // already here
	}}
	lib := Scan([]config.Library{{Name: "Media", Path: root, Kind: "auto"}}, []string{"Deluge"}, pending)

	titles := map[string]int{}
	for _, s := range lib.Shows {
		titles[s.Title] = len(s.Episodes)
	}
	if titles["Star Trek DS9"] != 3 || titles["Baseball (1994)"] != 2 || len(lib.Shows) != 2 {
		t.Fatalf("shows = %v", titles)
	}
	if len(lib.Movies) != 2 {
		t.Fatalf("movies = %d, want 2 (Office Space, Harold and Maude)", len(lib.Movies))
	}

	ds9 := lib.ShowByID(ID("show|Star Trek DS9"))
	statuses := [3]Status{ds9.Episodes[0].Status, ds9.Episodes[1].Status, ds9.Episodes[2].Status}
	if statuses != [3]Status{Ready, Ready, Downloading} {
		t.Fatalf("DS9 statuses = %v, want ready, ready, downloading", statuses)
	}

	// The same episode gets the same ID regardless of where its file lives.
	if ds9.Episodes[0].ID != ID("ep|star trek ds9|1|1") {
		t.Errorf("episode ID isn't derived from show/season/episode")
	}
	item, ok := lib.Lookup(ds9.Episodes[2].ID)
	if !ok || item.Ready {
		t.Errorf("downloading episode should be found but not ready: %+v", item)
	}
	if next := lib.Next(ds9.Episodes[0]); next != ds9.Episodes[1] {
		t.Errorf("Next(e01) = %+v", next)
	}
}

func TestExtrasBesideCodedEpisodes(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "The War (2007)/Season 01/The War (2007) - S01E01.mp4")
	touch(t, root, "The War (2007)/Season 01/The War (2007) - S01E02.mp4")
	touch(t, root, "The War (2007)/Extras/Interviews.mp4")
	touch(t, root, "The War (2007)/Extras/Making Of.mp4")
	lib := Scan([]config.Library{{Path: root, Kind: "tv"}}, nil, nil)

	if len(lib.Shows) != 1 {
		t.Fatalf("shows = %d, want 1", len(lib.Shows))
	}
	eps := lib.Shows[0].Episodes
	if len(eps) != 4 {
		t.Fatalf("episodes = %d, want 4 (2 episodes + 2 extras)", len(eps))
	}
	if eps[0].Season != 1 || eps[1].Season != 1 || eps[2].Season != 0 || eps[3].Season != 0 {
		t.Errorf("want episodes (season 1) then extras (season 0): %+v %+v %+v %+v", eps[0], eps[1], eps[2], eps[3])
	}
	if next := lib.Next(eps[1]); next != nil {
		t.Errorf("Next(last episode) = %+v, want nil rather than an extra", next)
	}
}
