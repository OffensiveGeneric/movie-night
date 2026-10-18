// Package state remembers what was watched, for "continue watching" and the
// restart button. It's per machine and kept next to the config file.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Watched struct {
	Show    string    `json:"show,omitempty"` // empty for movies
	Episode string    `json:"episode"`        // episode or movie ID
	At      time.Time `json:"at"`
}

type data struct {
	Shows           map[string]Watched `json:"shows"` // show ID -> last episode played
	Last            *Watched           `json:"last,omitempty"`
	DismissedInvite time.Time          `json:"dismissedInvite"`
}

type State struct {
	path string
	mu   sync.Mutex
	d    data
}

func Load(path string) *State {
	s := &State{path: path, d: data{Shows: map[string]Watched{}}}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &s.d)
	}
	if s.d.Shows == nil {
		s.d.Shows = map[string]Watched{}
	}
	return s
}

func (s *State) save() {
	os.MkdirAll(filepath.Dir(s.path), 0o755)
	b, _ := json.MarshalIndent(s.d, "", "  ")
	os.WriteFile(s.path, b, 0o644)
}

// Played records that an item was started. show is "" for movies.
func (s *State) Played(show, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := Watched{Show: show, Episode: id, At: time.Now()}
	if show != "" {
		s.d.Shows[show] = w
	}
	s.d.Last = &w
	s.save()
}

// Recent lists the last episode played per show, newest first.
func (s *State) Recent() []Watched {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Watched, 0, len(s.d.Shows))
	for _, w := range s.d.Shows {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Last is the most recently played item, if any.
func (s *State) Last() *Watched {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Last
}

func (s *State) DismissInvite(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.DismissedInvite = at
	s.save()
}

func (s *State) InviteDismissed(at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !at.After(s.d.DismissedInvite)
}
