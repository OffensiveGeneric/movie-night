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
	Server    string // host:port, or "" if unset
	MediaDirs []string
	values    map[string]string // client_settings, lower-cased keys
}

// bom is the UTF-8 byte order mark some editors put at the start of the file.
const bom = string(rune(0xFEFF))

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
	server := map[string]string{}
	section := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(b), bom), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch section {
		case "client_settings":
			s.values[key] = value
		case "server_data":
			server[key] = value
		}
	}
	s.Name = s.values["name"]
	s.Room = s.values["room"]
	// Syncplay stores either host = "example.org:8999" or host and port apart.
	if h := server["host"]; h != "" && h != "None" {
		s.Server = h
		if p := server["port"]; p != "" && !strings.Contains(h, ":") {
			s.Server = h + ":" + p
		}
	}
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
func Fix(path string) error { return Set(path, "client_settings", Recommended) }

// FormatList writes paths the way Syncplay stores mediasearchdirectories: a
// Python list literal with backslashes escaped.
func FormatList(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		p = strings.ReplaceAll(p, `\`, `\\`)
		quoted[i] = "'" + strings.ReplaceAll(p, "'", `\'`) + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Set writes key = value pairs into one section of the settings file,
// replacing existing keys and adding missing ones (and the section, and the
// file) as needed. Every other line is left as it was.
func Set(path, section string, values []Setting) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := strings.TrimPrefix(string(b), bom)
	newline := "\n"
	if strings.Contains(text, "\r\n") || (text == "" && runtime.GOOS == "windows") {
		newline = "\r\n"
	}
	var lines []string
	if text != "" {
		lines = strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	}

	pending := map[string]string{}
	for _, v := range values {
		pending[strings.ToLower(v.Key)] = v.Value
	}
	current, sectionEnd := "", -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			current = strings.ToLower(t[1 : len(t)-1])
			if current == section {
				sectionEnd = i
			}
			continue
		}
		if current != section {
			continue
		}
		if t != "" {
			sectionEnd = i
		}
		key, _, ok := strings.Cut(t, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if v, want := pending[key]; ok && want {
			lines[i] = key + " = " + v
			delete(pending, key)
		}
	}

	var extra []string
	for _, v := range values {
		if val, ok := pending[strings.ToLower(v.Key)]; ok {
			extra = append(extra, strings.ToLower(v.Key)+" = "+val)
		}
	}
	if len(extra) > 0 {
		if sectionEnd < 0 {
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, "["+section+"]")
			sectionEnd = len(lines) - 1
		}
		lines = append(lines[:sectionEnd+1], append(extra, lines[sectionEnd+1:]...)...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, newline)), 0o644)
}
