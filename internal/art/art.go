// Package art keeps a folder of posters and episode stills named by library
// item ID, and fills it from TMDB in the background.
//
// When the folder sits inside a Syncthing share, the machine with a TMDB key
// fetches the images and the other machine simply receives them, so only one
// of the two people needs a key.
package art

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/tmdb"
)

var validID = regexp.MustCompile(`^[0-9a-f]{12}$`)

type Store struct {
	Dir string

	mu     sync.Mutex
	misses map[string]time.Time // item ID -> when TMDB had nothing for it
}

func NewStore(dir string) *Store {
	s := &Store{Dir: dir, misses: map[string]time.Time{}}
	if b, err := os.ReadFile(s.missesPath()); err == nil {
		json.Unmarshal(b, &s.misses)
	}
	return s
}

// Path returns the image file for an item ID, or "" for an invalid ID.
func (s *Store) Path(id string) string {
	if !validID.MatchString(id) {
		return ""
	}
	return filepath.Join(s.Dir, id+".jpg")
}

func (s *Store) Has(id string) bool {
	p := s.Path(id)
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func (s *Store) missesPath() string { return filepath.Join(s.Dir, "misses.json") }

// recentlyMissed avoids asking TMDB again for a week about something it
// didn't have.
func (s *Store) recentlyMissed(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.misses[id]
	return ok && time.Since(t) < 7*24*time.Hour
}

func (s *Store) miss(id string) {
	s.mu.Lock()
	s.misses[id] = time.Now()
	s.mu.Unlock()
}

func (s *Store) saveMisses() {
	s.mu.Lock()
	b, _ := json.MarshalIndent(s.misses, "", " ")
	s.mu.Unlock()
	writeAtomic(s.missesPath(), b)
}

func (s *Store) put(id string, data []byte) error {
	return writeAtomic(s.Path(id), data)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type Progress struct {
	Running bool `json:"running"`
	Done    int  `json:"done"`
	Total   int  `json:"total"`
}

type Fetcher struct {
	Client    *tmdb.Client
	Store     *Store
	Overrides map[string]string // title -> "tv:580" / "movie:1234"

	mu       sync.Mutex
	progress Progress
}

func (f *Fetcher) Progress() Progress {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.progress
}

var titleYear = regexp.MustCompile(`^(.*?)\s*\((\d{4})\)$`)

func splitYear(title string) (string, int) {
	if m := titleYear.FindStringSubmatch(title); m != nil {
		y, _ := strconv.Atoi(m[2])
		return m[1], y
	}
	return title, 0
}

// Run fetches whatever art is missing for lib. Only one run happens at a
// time; a second call while one is running returns immediately.
func (f *Fetcher) Run(ctx context.Context, lib *library.Library) {
	f.mu.Lock()
	if f.progress.Running {
		f.mu.Unlock()
		return
	}
	f.progress = Progress{Running: true, Total: len(lib.Shows) + len(lib.Movies)}
	f.mu.Unlock()
	defer func() {
		f.Store.saveMisses()
		f.mu.Lock()
		f.progress.Running = false
		f.mu.Unlock()
	}()

	for _, s := range lib.Shows {
		if ctx.Err() != nil {
			return
		}
		if err := f.show(ctx, s); err != nil {
			log.Printf("art: %s: %v", s.Title, err)
		}
		f.step()
	}
	for _, m := range lib.Movies {
		if ctx.Err() != nil {
			return
		}
		if err := f.movie(ctx, m); err != nil {
			log.Printf("art: %s: %v", m.Title, err)
		}
		f.step()
	}
}

func (f *Fetcher) step() {
	f.mu.Lock()
	f.progress.Done++
	f.mu.Unlock()
}

func (f *Fetcher) needs(id string) bool { return !f.Store.Has(id) && !f.Store.recentlyMissed(id) }

// resolve finds a TMDB entry by pinned override or by search.
func (f *Fetcher) resolve(ctx context.Context, kind, title string, year int) (*tmdb.Result, error) {
	for k, v := range f.Overrides {
		if !strings.EqualFold(k, title) {
			continue
		}
		okind, sid, _ := strings.Cut(v, ":")
		id, err := strconv.Atoi(sid)
		if err != nil {
			break
		}
		return f.Client.Details(ctx, okind, id)
	}
	r, err := f.Client.Search(ctx, kind, title, year)
	if err != nil || r != nil {
		return r, err
	}
	// "Director - Title" names: try the part after the dash.
	if _, rest, ok := strings.Cut(title, " - "); ok {
		return f.Client.Search(ctx, kind, rest, year)
	}
	return nil, nil
}

func (f *Fetcher) show(ctx context.Context, s *library.Show) error {
	var wantStills []*library.Episode
	for _, e := range s.Episodes {
		// Extras (season 0 here) don't line up with TMDB's specials.
		if e.Season != 0 && f.needs(e.ID) {
			wantStills = append(wantStills, e)
		}
	}
	if !f.needs(s.ID) && len(wantStills) == 0 {
		return nil
	}

	title, year := splitYear(s.Title)
	r, err := f.resolve(ctx, "tv", title, year)
	if err != nil {
		return err
	}
	if r == nil {
		f.Store.miss(s.ID)
		for _, e := range wantStills {
			f.Store.miss(e.ID)
		}
		return nil
	}
	if f.needs(s.ID) {
		f.save(ctx, s.ID, "w342", r.PosterPath)
	}

	seasons := map[int][]*library.Episode{}
	for _, e := range wantStills {
		seasons[e.Season] = append(seasons[e.Season], e)
	}
	for n, eps := range seasons {
		list, err := f.Client.Season(ctx, r.ID, n)
		if err != nil {
			log.Printf("art: %s season %d: %v", s.Title, n, err)
			continue
		}
		stills := map[int]string{}
		for _, te := range list {
			stills[te.EpisodeNumber] = te.StillPath
		}
		for _, e := range eps {
			f.save(ctx, e.ID, "w300", stills[e.Number])
		}
	}
	return nil
}

func (f *Fetcher) movie(ctx context.Context, m *library.Movie) error {
	if !f.needs(m.ID) {
		return nil
	}
	r, err := f.resolve(ctx, "movie", m.Title, m.Year)
	if err != nil {
		return err
	}
	if r == nil {
		f.Store.miss(m.ID)
		return nil
	}
	f.save(ctx, m.ID, "w342", r.PosterPath)
	return nil
}

func (f *Fetcher) save(ctx context.Context, id, size, path string) {
	if path == "" {
		f.Store.miss(id)
		return
	}
	data, err := f.Client.Image(ctx, size, path)
	if err != nil {
		log.Printf("art: image %s: %v", path, err)
		return
	}
	if err := f.Store.put(id, data); err != nil {
		log.Printf("art: saving %s: %v", id, err)
	}
}
