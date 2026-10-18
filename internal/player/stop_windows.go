package player

import "os/exec"

// stopPrevious ends Syncplay and the VLC instance it launched (recognisable by
// its Syncplay Lua interface argument), leaving any other VLC windows alone.
func stopPrevious() {
	exec.Command("powershell", "-NoProfile", "-Command", `
		Get-Process Syncplay -ErrorAction SilentlyContinue | Stop-Process -Force
		Get-CimInstance Win32_Process -Filter "Name = 'vlc.exe'" |
			Where-Object { $_.CommandLine -match 'syncplay' } |
			ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`).Run()
}
