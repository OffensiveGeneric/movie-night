// Package syncplay reads and repairs Syncplay's own settings file.
package syncplay

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ConfigPath is where Syncplay keeps its settings: %APPDATA%\syncplay.ini on
// Windows, ~/.syncplay elsewhere.
func ConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "syncplay.ini")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".syncplay")
}

// Recommended client settings for two people watching local copies:
//   - readyatstart / unpauseaction: whoever presses play starts it for both,
//     instead of the "not ready" dance.
//   - sharedplaylistenabled / foldersearchdoublecheckinterval: Syncplay
//     re-scans its media folders every 30 seconds and, with shared playlists
//     on, can re-open the current file when the folder changed. While
//     Syncthing is adding files to that folder, that sends playback back to
//     00:00 every 30 seconds. Movie Night opens files itself, so neither is
//     needed.
var Recommended = []Setting{
	{"readyatstart", "True"},
	{"unpauseaction", "Always"},
	{"sharedplaylistenabled", "False"},
	{"foldersearchdoublecheckinterval", "86400.0"},
}

type Setting struct{ Key, Value string }

type Settings struct {
	Name      string
	Room      string
	MediaDirs []string
	values    map[string]string // client_settings, lower-cased keys
}

var listItem = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'`)

// Read parses the settings file. A missing file yields empty settings.
func Read(path string) (*Settings, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Settings{values: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	s := &Settings{values: map[string]string{}}
	section := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section != "client_settings" {
			continue
		}
		s.values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	s.Name = s.values["name"]
	s.Room = s.values["room"]
	// mediasearchdirectories is a Python list literal: ['C:\\TV', 'D:\\Movies']
	for _, m := range listItem.FindAllStringSubmatch(s.values["mediasearchdirectories"], -1) {
		s.MediaDirs = append(s.MediaDirs, strings.ReplaceAll(m[1], `\\`, `\`))
	}
	return s, nil
}

// Problems lists recommended settings that aren't set as recommended.
func (s *Settings) Problems() []Setting {
	var out []Setting
	for _, r := range Recommended {
		if !strings.EqualFold(s.values[r.Key], r.Value) {
			out = append(out, r)
		}
	}
	return out
}

// Fix rewrites the settings file with the recommended values, leaving every
// other line alone. Syncplay needs restarting to pick them up.
func Fix(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	pending := map[string]string{}
	for _, r := range Recommended {
		pending[r.Key] = r.Value
	}
	section, sectionEnd := "", -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = strings.ToLower(t[1 : len(t)-1])
			continue
		}
		if section != "client_settings" {
			continue
		}
		sectionEnd = i
		key, _, ok := strings.Cut(t, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if v, want := pending[key]; ok && want {
			lines[i] = key + " = " + v
			delete(pending, key)
		}
	}

	var extra []string
	for _, r := range Recommended {
		if v, ok := pending[r.Key]; ok {
			extra = append(extra, r.Key+" = "+v)
		}
	}
	if len(extra) > 0 {
		if sectionEnd < 0 {
			lines = append(lines, "[client_settings]")
			sectionEnd = len(lines) - 1
		}
		lines = append(lines[:sectionEnd+1], append(extra, lines[sectionEnd+1:]...)...)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, newline)), 0o644)
}
