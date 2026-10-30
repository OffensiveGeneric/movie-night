// Package player opens a file in Syncplay, which starts the media player and
// joins the shared room configured in Syncplay's own settings.
package player

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

type Launcher struct {
	// Syncplay is the executable path; empty means search the usual places.
	Syncplay string
}

// Play closes any earlier Syncplay session (and the player it controls), so
// the room never sees us twice, then opens path in a new one.
func (l Launcher) Play(path string) error {
	exe := l.Syncplay
	if exe == "" {
		exe = FindSyncplay()
	}
	if exe == "" {
		return errors.New("Syncplay not found; set \"syncplay\" in the config")
	}
	stopPrevious()
	cmd := exec.Command(exe, path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// FindSyncplay returns Syncplay's path in the usual install locations, or "".
func FindSyncplay() string {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			os.Getenv("ProgramFiles(x86)") + `\Syncplay\Syncplay.exe`,
			os.Getenv("ProgramFiles") + `\Syncplay\Syncplay.exe`,
		}
	case "darwin":
		candidates = []string{"/Applications/Syncplay.app/Contents/MacOS/Syncplay"}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("syncplay"); err == nil {
		return p
	}
	return ""
}
