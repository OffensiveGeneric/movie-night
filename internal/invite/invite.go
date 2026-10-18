// Package invite passes "I'm starting this, join me" from the host to the
// guest. The host writes a small JSON file into a folder Syncthing already
// shares with the guest; a few seconds later it appears on the guest's
// machine. No server, no ports.
package invite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Invite struct {
	ID    string    `json:"id"`    // library item ID, the same on both machines
	Title string    `json:"title"` // for display
	From  string    `json:"from"`
	At    time.Time `json:"at"`
}

// Fresh reports whether an invite is recent enough to still be worth joining.
func (i *Invite) Fresh() bool { return time.Since(i.At) < 3*time.Hour }

func Write(path string, inv Invite) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(inv, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Read returns the current invite, or nil if there is none.
func Read(path string) (*Invite, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inv := &Invite{}
	return inv, json.Unmarshal(b, inv)
}
