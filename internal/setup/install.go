package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallDir is where Movie Night installs itself:
// %LOCALAPPDATA%\MovieNight on Windows.
func InstallDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "MovieNight")
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "movienight", "bin")
}

// InstallSelf copies the running executable into InstallDir and, on
// Windows, puts a "Movie Night" shortcut on the desktop. It returns the
// installed path.
func (e *Env) InstallSelf() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "movienight"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(InstallDir(), name)
	if !samePath(self, dest) {
		if err := e.step("install Movie Night to "+dest, func() error { return copyFile(self, dest) }); err != nil {
			return "", err
		}
	}
	if runtime.GOOS == "windows" {
		if err := e.step(`put a "Movie Night" shortcut on the desktop`, func() error { return shortcut(dest) }); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// shortcut creates Desktop\Movie Night.lnk through Windows' own shortcut API.
func shortcut(target string) error {
	script := fmt.Sprintf(`$s = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path ([Environment]::GetFolderPath('Desktop')) 'Movie Night.lnk'))
$s.TargetPath = '%s'
$s.WorkingDirectory = '%s'
$s.Description = 'Watch together'
$s.Save()`, psQuote(target), psQuote(filepath.Dir(target)))
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// psQuote escapes a value for a single-quoted PowerShell string.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }
