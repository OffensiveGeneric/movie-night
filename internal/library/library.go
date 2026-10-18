// Package library scans media folders and groups what it finds into shows,
// episodes and movies.
package library

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/offensivegeneric/movie-night/internal/config"
)

var videoExt = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true, ".wmv": true,
}

type Episode struct {
	ID     string `json:"id"`
	Season int    `json:"season"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Size   int64  `json:"size"`
	path   string
}

type Show struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Episodes []*Episode `json:"episodes"`
}

type Movie struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
	Size  int64  `json:"size"`
	path  string
}

type Library struct {
	Shows  []*Show           `json:"shows"`
	Movies []*Movie          `json:"movies"`
	paths  map[string]string // item ID -> file path
}

// Path returns the file behind an episode or movie ID.
func (l *Library) Path(id string) (string, bool) {
	p, ok := l.paths[id]
	return p, ok
}

// Title returns a display name for an episode or movie ID.
func (l *Library) Title(id string) string {
	for _, s := range l.Shows {
		for _, e := range s.Episodes {
			if e.ID == id {
				return s.Title + " " + episodeCode(e)
			}
		}
	}
	for _, m := range l.Movies {
		if m.ID == id {
			return m.Title
		}
	}
	return ""
}

func episodeCode(e *Episode) string {
	return fmt.Sprintf("S%02dE%02d", e.Season, e.Number)
}

func idFor(path string) string {
	sum := sha1.Sum([]byte(strings.ToLower(path)))
	return hex.EncodeToString(sum[:6])
}

type found struct {
	path string
	rel  string // relative to the library root, with forward slashes
	size int64
}

// Scan walks every library and builds the catalogue. Folders that can't be
// read are skipped rather than failing the whole scan.
func Scan(libs []config.Library, exclude []string) (*Library, error) {
	skip := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		skip[strings.ToLower(name)] = true
	}

	lib := &Library{paths: map[string]string{}}
	shows := map[string]*Show{}
	seen := map[string]bool{} // dedupe identical episodes/movies found twice

	for _, l := range libs {
		var files []found
		filepath.WalkDir(l.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if path != l.Path && (skip[strings.ToLower(name)] || strings.HasPrefix(name, ".")) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !videoExt[strings.ToLower(filepath.Ext(name))] ||
				strings.Contains(strings.ToLower(name), "sample") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(l.Path, path)
			files = append(files, found{path, filepath.ToSlash(rel), info.Size()})
			return nil
		})

		switch l.Kind {
		case "tv":
			addEpisodes(lib, shows, seen, files)
		case "movies":
			addMovies(lib, seen, files)
		}
	}

	for _, s := range shows {
		sort.Slice(s.Episodes, func(i, j int) bool {
			a, b := s.Episodes[i], s.Episodes[j]
			if a.Season != b.Season {
				return a.Season < b.Season
			}
			return a.Number < b.Number
		})
		lib.Shows = append(lib.Shows, s)
	}
	sort.Slice(lib.Shows, func(i, j int) bool { return lib.Shows[i].Title < lib.Shows[j].Title })
	sort.Slice(lib.Movies, func(i, j int) bool { return lib.Movies[i].Title < lib.Movies[j].Title })
	return lib, nil
}

func addEpisodes(lib *Library, shows map[string]*Show, seen map[string]bool, files []found) {
	var unnumbered []found
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
		p, ok := parseEpisode(base)
		if !ok {
			unnumbered = append(unnumbered, f)
			continue
		}
		key := strings.ToLower(p.show) + "|" + strconv.Itoa(p.season) + "|" + strconv.Itoa(p.episode)
		if seen[key] {
			continue
		}
		seen[key] = true
		addEpisode(lib, showFor(shows, p.show), p.season, p.episode, p.title, f)
	}

	// Files without an episode code (documentary series and the like) are
	// grouped by their top-level folder and numbered in file-name order.
	sort.Slice(unnumbered, func(i, j int) bool { return unnumbered[i].rel < unnumbered[j].rel })
	counts := map[*Show]int{}
	for _, f := range unnumbered {
		top, _, nested := strings.Cut(f.rel, "/")
		if !nested {
			top = strings.TrimSuffix(top, filepath.Ext(top))
		}
		title, year := parseMovie(top)
		if year != 0 {
			title += " (" + strconv.Itoa(year) + ")"
		}
		s := showFor(shows, title)
		counts[s]++
		base := strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
		addEpisode(lib, s, 1, counts[s], clean(base), f)
	}
}

func showFor(shows map[string]*Show, title string) *Show {
	key := strings.ToLower(title)
	s, ok := shows[key]
	if !ok {
		s = &Show{ID: idFor("show:" + key), Title: title}
		shows[key] = s
	}
	return s
}

func addEpisode(lib *Library, s *Show, season, number int, title string, f found) {
	e := &Episode{ID: idFor(f.path), Season: season, Number: number, Title: title, Size: f.size, path: f.path}
	s.Episodes = append(s.Episodes, e)
	lib.paths[e.ID] = f.path
}

func addMovies(lib *Library, seen map[string]bool, files []found) {
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
		title, year := parseMovie(base)
		// A film inside its own folder is usually better named by the folder.
		if top, _, nested := strings.Cut(f.rel, "/"); nested {
			if t, y := parseMovie(top); y != 0 || year == 0 {
				title, year = t, y
			}
		}
		key := strings.ToLower(title) + "|" + strconv.Itoa(year)
		if seen[key] {
			continue
		}
		seen[key] = true
		m := &Movie{ID: idFor(f.path), Title: title, Year: year, Size: f.size, path: f.path}
		lib.Movies = append(lib.Movies, m)
		lib.paths[m.ID] = f.path
	}
}
