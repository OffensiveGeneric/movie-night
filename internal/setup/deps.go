package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/offensivegeneric/movie-night/internal/player"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
)

// Dep is a program Movie Night drives. It's installed from its official
// winget package, never bundled, so each keeps its own license and updates.
type Dep struct {
	Name      string
	WingetID  string
	UserScope bool          // install for this user only
	Find      func() string // installed path, or ""
}

var Deps = []Dep{
	// Syncthing Windows Setup runs Syncthing in the background at login,
	// which is what we want for downloads that take a day.
	{"Syncthing", "BillStewart.SyncthingWindowsSetup", true, func() string {
		if b := syncthing.Binaries(); len(b) > 0 {
			return b[0]
		}
		return ""
	}},
	{"Syncplay", "Syncplay.Syncplay", false, player.FindSyncplay},
	{"VLC", "VideoLAN.VLC", false, FindVLC},
}

// FindVLC returns VLC's path, or "".
func FindVLC() string {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			filepath.Join(os.Getenv("ProgramFiles"), "VideoLAN", "VLC", "vlc.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "VideoLAN", "VLC", "vlc.exe"),
		}
	case "darwin":
		candidates = []string{"/Applications/VLC.app/Contents/MacOS/VLC"}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("vlc"); err == nil {
		return p
	}
	return ""
}

// EnsureDeps installs whatever is missing, like a package manager resolving
// dependencies. It returns the found paths by name.
func EnsureDeps(ctx context.Context, out io.Writer, dryRun bool) (map[string]string, error) {
	paths := map[string]string{}
	for _, d := range Deps {
		if p := d.Find(); p != "" {
			fmt.Fprintf(out, "  ✓ %s is installed\n", d.Name)
			paths[d.Name] = p
			continue
		}
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("%s isn't installed; install it with your package manager and run setup again", d.Name)
		}
		if dryRun {
			fmt.Fprintf(out, "  • would install %s (winget %s)\n", d.Name, d.WingetID)
			continue
		}
		fmt.Fprintf(out, "  • installing %s…\n", d.Name)
		args := []string{"install", "--id", d.WingetID, "--exact", "--silent",
			"--accept-package-agreements", "--accept-source-agreements"}
		if d.UserScope {
			args = append(args, "--scope", "user")
		}
		cmd := exec.CommandContext(ctx, "winget", args...)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		runErr := cmd.Run()
		p := d.Find()
		if p == "" {
			if runErr == nil {
				runErr = fmt.Errorf("installed but not found")
			}
			return nil, fmt.Errorf("installing %s: %w (try: winget install --id %s)", d.Name, runErr, d.WingetID)
		}
		fmt.Fprintf(out, "  ✓ %s installed\n", d.Name)
		paths[d.Name] = p
	}
	return paths, nil
}
