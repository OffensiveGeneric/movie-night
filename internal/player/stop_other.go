//go:build !windows

package player

import "os/exec"

// stopPrevious ends Syncplay and the player instance it launched.
func stopPrevious() {
	exec.Command("pkill", "-x", "Syncplay").Run()
	exec.Command("pkill", "-f", "lua-intf=syncplay").Run()
}
