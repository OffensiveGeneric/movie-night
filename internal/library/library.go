// Package library scans media folders and groups what it finds into shows,
// episodes and movies.
//
// Every item's ID is derived from what it is (show + season + episode, or
// title + year), not from where the file lives. The same episode therefore
// has the same ID on both machines, which is what lets an invite, a poster
// or "continue watching" refer to it, and an episode keeps its ID when it
// goes from downloading to ready.
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

// IsVideo reports whether a file name has a video extension.
func IsVideo(name string) bool { return videoExt[strings.ToLower(filepath.Ext(name))] }

type Status string

const (
	Ready       Status = "ready"
	Downloading Status = "downloading"
	Queued      Status = "queued"
)

type Episode struct {
	ID     string `json:"id"`
	Show   string `json:"show"` // show ID
	Season int    `json:"season"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Size   int64  `json:"size"`
	Status Status `json:"status"`
	path   string
}

type Show struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Episodes []*Episode `json:"episodes"`
}

type Movie struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Year   int    `json:"year,omitempty"`
	Size   int64  `json:"size"`
	Status Status `json:"status"`
	path   string
}

type Library struct {
	Shows  []*Show  `json:"shows"`
	Movies []*Movie `json:"movies"`

	episodes map[string]*Episode
	shows    map[string]*Show
	movies   map[string]*Movie
}

// Pending is a file Syncthing knows about but hasn't finished downloading.
type Pending struct {
	Rel    string // relative to the library root, forward slashes
	Size   int64
	Status Status
}

// Item is what the server needs to play something.
type Item struct {
	ID, Title, Path string
	Ready           bool
	Episode         *Episode // nil for movies
}

// Lookup finds an episode or movie by ID.
func (l *Library) Lookup(id string) (Item, bool) {
	if e, ok := l.episodes[id]; ok {
		s := l.shows[e.Show]
		return Item{ID: id, Title: s.Title + " " + EpisodeCode(e), Path: e.path, Ready: e.Status == Ready, Episode: e}, true
	}
	if m, ok := l.movies[id]; ok {
		return Item{ID: id, Title: m.Title, Path: m.path, Ready: m.Status == Ready}, true
	}
	return Item{}, false
}

// ShowByID returns a show, or nil.
func (l *Library) ShowByID(id string) *Show { return l.shows[id] }

// Next returns the episode after e in its show, or nil at the end. Extras
// only follow other extras.
func (l *Library) Next(e *Episode) *Episode {
	eps := l.shows[e.Show].Episodes
	for i, x := range eps {
		if x == e && i+1 < len(eps) && (eps[i+1].Season != 0 || e.Season == 0) {
			return eps[i+1]
		}
	}
	return nil
}

func EpisodeCode(e *Episode) string {
	return fmt.Sprintf("S%02dE%02d", e.Season, e.Number)
}

// ID hashes a stable item key ("ep|star trek ds9|1|12", "movie|office space|1999").
func ID(key string) string {
	sum := sha1.Sum([]byte(strings.ToLower(spaces.ReplaceAllString(key, " "))))
	return hex.EncodeToString(sum[:6])
}

type found struct {
	path   string // empty for files that haven't arrived yet
	rel    string // relative to the library root, with forward slashes
	size   int64
	status Status
}

// Scan walks every library and builds the catalogue. pending maps a library
// root to files that are still on their way; they're listed but not
// playable. Folders that can't be read are skipped rather than failing the
// whole scan.
func Scan(libs []config.Library, exclude []string, pending map[string][]Pending) *Library {
	skip := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		skip[strings.ToLower(name)] = true
	}
	lib := &Library{episodes: map[string]*Episode{}, shows: map[string]*Show{}, movies: map[string]*Movie{}}

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
			if d.IsDir() || !IsVideo(name) || strings.Contains(strings.ToLower(name), "sample") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(l.Path, path)
			files = append(files, found{path, filepath.ToSlash(rel), info.Size(), Ready})
			return nil
		})
		for _, p := range pending[l.Path] {
			if excluded(p.Rel, skip) || !IsVideo(p.Rel) {
				continue
			}
			files = append(files, found{"", p.Rel, p.Size, p.Status})
		}
		lib.add(l.Kind, files)
	}

	for _, s := range lib.shows {
		sort.Slice(s.Episodes, func(i, j int) bool {
			a, b := s.Episodes[i], s.Episodes[j]
			if a.Season != b.Season {
				// Extras (season 0) go after the real seasons.
				return a.Season != 0 && (b.Season == 0 || a.Season < b.Season)
			}
			return a.Number < b.Number
		})
		lib.Shows = append(lib.Shows, s)
	}
	for _, m := range lib.movies {
		lib.Movies = append(lib.Movies, m)
	}
	sort.Slice(lib.Shows, func(i, j int) bool { return lib.Shows[i].Title < lib.Shows[j].Title })
	sort.Slice(lib.Movies, func(i, j int) bool { return lib.Movies[i].Title < lib.Movies[j].Title })
	return lib
}

func excluded(rel string, skip map[string]bool) bool {
	for _, part := range strings.Split(rel, "/") {
		if skip[strings.ToLower(part)] || strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func baseName(rel string) string {
	b := filepath.Base(rel)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// add sorts one library's files into episodes and movies. "tv" treats every
// file as an episode, "movies" every file as a movie, and "auto" uses episode
// codes, then folder shape: several uncoded videos in one folder are a series,
// a lone video is a movie.
func (lib *Library) add(kind string, files []found) {
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })

	perFolder := map[string]int{}    // top folder -> uncoded videos in it
	codedShow := map[string]string{} // top folder -> show named by its coded episodes
	for _, f := range files {
		top, _, nested := strings.Cut(f.rel, "/")
		if !nested {
			continue
		}
		if p, coded := parseEpisode(baseName(f.rel)); coded {
			if _, seen := codedShow[top]; !seen {
				codedShow[top] = p.show
			}
		} else {
			perFolder[top]++
		}
	}

	numbered := map[string]int{} // show -> uncoded episodes so far
	for _, f := range files {
		if kind == "movies" {
			lib.addMovie(f)
			continue
		}
		if p, ok := parseEpisode(baseName(f.rel)); ok {
			lib.addEpisode(p.show, p.season, p.episode, p.title, f)
			continue
		}
		top, _, nested := strings.Cut(f.rel, "/")
		// Uncoded files next to coded episodes are extras (featurettes, an
		// alternate copy) of that show: season 0, so they can't collide with
		// the real episode numbers.
		if show, ok := codedShow[top]; ok && nested {
			numbered[show+"|0"]++
			lib.addEpisode(show, 0, numbered[show+"|0"], clean(baseName(f.rel)), f)
			continue
		}
		if kind == "auto" && (!nested || perFolder[top] < 2) {
			lib.addMovie(f)
			continue
		}
		// Documentary series and the like: group by top-level folder and
		// number in file-name order.
		if !nested {
			top = baseName(top)
		}
		title, year := parseMovie(top)
		if year != 0 {
			title += " (" + strconv.Itoa(year) + ")"
		}
		numbered[title+"|1"]++
		lib.addEpisode(title, 1, numbered[title+"|1"], clean(baseName(f.rel)), f)
	}
}

func (lib *Library) addEpisode(show string, season, number int, title string, f found) {
	showID := ID("show|" + show)
	s, ok := lib.shows[showID]
	if !ok {
		s = &Show{ID: showID, Title: show}
		lib.shows[showID] = s
	}
	id := ID(fmt.Sprintf("ep|%s|%d|%d", show, season, number))
	if old, dup := lib.episodes[id]; dup {
		// Keep one copy, preferring a file that's actually here.
		if old.Status != Ready && f.status == Ready {
			old.path, old.Size, old.Status = f.path, f.size, Ready
		}
		return
	}
	e := &Episode{ID: id, Show: showID, Season: season, Number: number, Title: title, Size: f.size, Status: f.status, path: f.path}
	s.Episodes = append(s.Episodes, e)
	lib.episodes[id] = e
}

func (lib *Library) addMovie(f found) {
	title, year := parseMovie(baseName(f.rel))
	// A film inside its own folder is usually better named by the folder.
	if top, _, nested := strings.Cut(f.rel, "/"); nested {
		if t, y := parseMovie(top); y != 0 || year == 0 {
			title, year = t, y
		}
	}
	id := ID(fmt.Sprintf("movie|%s|%d", title, year))
	if old, dup := lib.movies[id]; dup {
		if old.Status != Ready && f.status == Ready {
			old.path, old.Size, old.Status = f.path, f.size, Ready
		}
		return
	}
	lib.movies[id] = &Movie{ID: id, Title: title, Year: year, Size: f.size, Status: f.status, path: f.path}
}
