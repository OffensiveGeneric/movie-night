package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

// wingetNoUpgrade is winget's exit code for "already up to date"
// (APPINSTALLER_CLI_ERROR_UPDATE_NOT_APPLICABLE, 0x8A15002B).
const wingetNoUpgrade = 0x8A15002B

// UpdateDeps upgrades Syncthing, Syncplay and VLC through winget. It refuses
// while Syncplay or VLC is running: an update closes the player, which is
// exactly how a movie gets cut off halfway.
func UpdateDeps(ctx context.Context, out io.Writer, force bool) error {
	if runtime.GOOS != "windows" {
		return errors.New("update uses winget, which is Windows-only; update with your package manager")
	}
	if !force {
		for _, name := range []string{"Syncplay.exe", "vlc.exe"} {
			if running(name) {
				return fmt.Errorf("%s is open. Finish watching and close it first (or use -force)", strings.TrimSuffix(name, ".exe"))
			}
		}
	}
	for _, d := range Deps {
		if d.Find() == "" {
			fmt.Fprintf(out, "  • %s isn't installed; run \"movienight setup\" to install it\n", d.Name)
			continue
		}
		args := []string{"upgrade", "--id", d.WingetID, "--exact", "--silent",
			"--accept-package-agreements", "--accept-source-agreements"}
		if d.UserScope {
			args = append(args, "--scope", "user")
		}
		cmd := exec.CommandContext(ctx, "winget", args...)
		output, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		switch {
		case err == nil && strings.Contains(string(output), "No available upgrade"):
			fmt.Fprintf(out, "  ✓ %s is up to date\n", d.Name)
		case err == nil:
			fmt.Fprintf(out, "  ✓ %s updated\n", d.Name)
		case errors.As(err, &exit) && uint32(exit.ExitCode()) == wingetNoUpgrade:
			fmt.Fprintf(out, "  ✓ %s is up to date\n", d.Name)
		default:
			fmt.Fprintf(out, "  ! %s: %v (try: winget upgrade --id %s)\n", d.Name, err, d.WingetID)
		}
	}
	return nil
}

// running reports whether a process with this image name is running.
func running(image string) bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/NH").Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), strings.ToLower(image))
}
