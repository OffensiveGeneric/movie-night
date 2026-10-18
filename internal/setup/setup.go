// Package setup installs and configures everything Movie Night drives, so a
// new machine goes from nothing to paired in one command:
//
//	movienight setup host           on the computer with the library
//	movienight setup join <code>    on the other one
//
// Like a package manager it installs missing dependencies (from their
// official winget packages) and is safe to re-run: existing Syncthing
// folders and devices are only ever extended, never changed.
package setup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/syncplay"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
)

// Env is where setup writes. Tests point it at temp files and a fake
// Syncthing; DryRun prints what would change instead of changing it.
type Env struct {
	Out         io.Writer
	Syncthing   *syncthing.Client
	SyncplayIni string
	ConfigPath  string
	VLC         string
	DryRun      bool
}

func (e *Env) say(format string, a ...any) { fmt.Fprintf(e.Out, format+"\n", a...) }

// step runs a change, or describes it in a dry run.
func (e *Env) step(what string, fn func() error) error {
	if e.DryRun {
		e.say("  • would %s", what)
		return nil
	}
	if err := fn(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	e.say("  ✓ %s", what)
	return nil
}

type HostOptions struct {
	Name      string
	Libraries []string
	Server    string // Syncplay server host:port
	Room      string
}

// Host shares the libraries send-only and returns the code for the guest.
// Libraries Syncthing already shares are reused as they are.
func (e *Env) Host(ctx context.Context, o HostOptions) (Code, error) {
	myID, err := e.Syncthing.MyID(ctx)
	if err != nil {
		return Code{}, fmt.Errorf("talking to Syncthing: %w", err)
	}
	folders, err := e.Syncthing.Folders(ctx)
	if err != nil {
		return Code{}, err
	}
	used := map[string]bool{}
	for _, f := range folders {
		used[f.ID] = true
	}

	code := Code{Device: myID, Name: o.Name, Server: o.Server, Room: o.Room}
	for _, lib := range o.Libraries {
		if m, ok := syncthing.Locate(folders, lib); ok {
			e.say("  ✓ %s is already shared in Syncthing as %q", lib, m.Folder.Label)
			code.Folders = append(code.Folders, CodeFolder{m.Folder.ID, m.Folder.Label})
			continue
		}
		base := filepath.Base(lib)
		id := uniqueID("movie-night-"+slug(base), used)
		spec := syncthing.FolderSpec{ID: id, Label: "Movie Night - " + base, Path: lib, Type: "sendonly"}
		if err := e.step(fmt.Sprintf("share %s in Syncthing (send-only)", lib), func() error {
			return e.Syncthing.EnsureFolder(ctx, spec)
		}); err != nil {
			return Code{}, err
		}
		code.Folders = append(code.Folders, CodeFolder{spec.ID, spec.Label})
	}
	if len(code.Folders) == 0 {
		return Code{}, fmt.Errorf("no library folders to share")
	}
	if err := e.configure(o.Name, o.Server, o.Room, o.Libraries); err != nil {
		return Code{}, err
	}
	return code, nil
}

type JoinOptions struct {
	Name string
	Dir  string // where new shared folders go, e.g. ~/Videos
}

// Join pairs with the host from its code: adds the host's device, creates
// matching receive-only folders, and points Syncplay at the same room. It
// returns this machine's device ID, which the host then accepts.
func (e *Env) Join(ctx context.Context, c Code, o JoinOptions) (string, error) {
	myID, err := e.Syncthing.MyID(ctx)
	if err != nil {
		return "", fmt.Errorf("talking to Syncthing: %w", err)
	}
	if c.Device == myID {
		return "", fmt.Errorf("this code was made on this computer; run join on the other one")
	}
	if err := e.step(fmt.Sprintf("add %s's computer to Syncthing", c.Name), func() error {
		return e.Syncthing.AddDevice(ctx, c.Device, c.Name)
	}); err != nil {
		return "", err
	}
	folders, err := e.Syncthing.Folders(ctx)
	if err != nil {
		return "", err
	}
	existing := map[string]syncthing.Folder{}
	for _, f := range folders {
		existing[f.ID] = f
	}

	var dirs []string
	for _, f := range c.Folders {
		path := filepath.Join(o.Dir, safeName(f.Label))
		what := fmt.Sprintf("receive %q into %s (receive-only, in order)", f.Label, path)
		if ex, ok := existing[f.ID]; ok {
			path = ex.Path // already set up here: keep it where it is
			what = fmt.Sprintf("keep receiving %q in %s (already set up)", f.Label, path)
		}
		spec := syncthing.FolderSpec{ID: f.ID, Label: f.Label, Path: path, Type: "receiveonly",
			Order: "alphabetic", Devices: []string{c.Device}}
		if err := e.step(what, func() error {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			return e.Syncthing.EnsureFolder(ctx, spec)
		}); err != nil {
			return "", err
		}
		dirs = append(dirs, path)
	}
	if err := e.configure(o.Name, c.Server, c.Room, dirs); err != nil {
		return "", err
	}
	return myID, nil
}

// configure points Syncplay at the room and the library folders, applies
// the settings that keep two players happy, and writes Movie Night's config.
func (e *Env) configure(name, server, room string, dirs []string) error {
	if server != "" {
		host, port, err := net.SplitHostPort(server)
		if err != nil {
			host, port = server, "8999"
		}
		if err := e.step(fmt.Sprintf("set Syncplay's server to %s", server), func() error {
			return syncplay.Set(e.SyncplayIni, "server_data", []syncplay.Setting{kv("host", host), kv("port", port)})
		}); err != nil {
			return err
		}
	}
	client := []syncplay.Setting{
		kv("name", name),
		kv("room", room),
		kv("mediasearchdirectories", syncplay.FormatList(dirs)),
		kv("forceguiprompt", "False"),
	}
	if e.VLC != "" {
		client = append(client, kv("playerpath", e.VLC))
	}
	client = append(client, syncplay.Recommended...)
	if err := e.step(fmt.Sprintf("set Syncplay's room, folders and player (room %s)", room), func() error {
		return syncplay.Set(e.SyncplayIni, "client_settings", client)
	}); err != nil {
		return err
	}

	var libs []config.Library
	for _, d := range dirs {
		libs = append(libs, config.Library{Name: filepath.Base(d), Path: d, Kind: "auto"})
	}
	return e.step("save Movie Night's settings", func() error {
		_, err := config.Update(e.ConfigPath, name, libs)
		return err
	})
}

func kv(key, value string) syncplay.Setting { return syncplay.Setting{Key: key, Value: value} }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		s = "library"
	}
	return s
}

func uniqueID(id string, used map[string]bool) string {
	candidate := id
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d", id, i)
	}
	used[candidate] = true
	return candidate
}

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*]`)

// safeName makes a folder label usable as a directory name.
func safeName(label string) string {
	if s := strings.TrimSpace(unsafeChars.ReplaceAllString(label, "-")); s != "" {
		return s
	}
	return "Movie Night"
}

// NewRoom makes a hard-to-guess Syncplay room name.
func NewRoom() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "movienight-" + hex.EncodeToString(b)
}
