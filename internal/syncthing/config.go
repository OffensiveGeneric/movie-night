package syncthing

import (
	"context"
	"net/url"
	"time"
)

// Config writes. Folders and devices are created from Syncthing's own
// defaults (GET /rest/config/defaults/...) so every field we don't care
// about keeps the value Syncthing would have picked, and then saved with PUT.
// Existing entries are only ever extended, never replaced.

// MyID is this device's ID.
func (c *Client) MyID(ctx context.Context) (string, error) {
	var s struct {
		MyID string `json:"myID"`
	}
	return s.MyID, c.get(ctx, "/rest/system/status", nil, &s)
}

// AddDevice adds a remote device unless it's already known.
func (c *Client) AddDevice(ctx context.Context, id, name string) error {
	path := "/rest/config/devices/" + url.PathEscape(id)
	var existing map[string]any
	err := c.get(ctx, path, nil, &existing)
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	dev := map[string]any{}
	if err := c.get(ctx, "/rest/config/defaults/device", nil, &dev); err != nil {
		return err
	}
	dev["deviceID"] = id
	dev["name"] = name
	dev["addresses"] = []string{"dynamic"}
	return c.do(ctx, "PUT", path, nil, dev, nil)
}

// FolderSpec is what Movie Night needs a shared folder to look like.
type FolderSpec struct {
	ID, Label, Path string
	Type            string   // "sendonly" or "receiveonly"
	Order           string   // pull order, e.g. "alphabetic"; "" keeps the default
	Devices         []string // remote device IDs to share with
}

// EnsureFolder creates the folder if it doesn't exist. If it does, it only
// adds any devices it isn't shared with yet; path and type are left alone.
func (c *Client) EnsureFolder(ctx context.Context, spec FolderSpec) error {
	path := "/rest/config/folders/" + url.PathEscape(spec.ID)
	var existing map[string]any
	err := c.get(ctx, path, nil, &existing)
	if err == nil {
		return c.shareWith(ctx, path, existing, spec.Devices)
	}
	if !isNotFound(err) {
		return err
	}
	f := map[string]any{}
	if err := c.get(ctx, "/rest/config/defaults/folder", nil, &f); err != nil {
		return err
	}
	f["id"], f["label"], f["path"], f["type"] = spec.ID, spec.Label, spec.Path, spec.Type
	if spec.Order != "" {
		f["order"] = spec.Order
	}
	var devices []any
	for _, d := range spec.Devices {
		devices = append(devices, map[string]any{"deviceID": d})
	}
	f["devices"] = devices
	return c.do(ctx, "PUT", path, nil, f, nil)
}

// ShareFolder adds a device to an existing folder's share list.
func (c *Client) ShareFolder(ctx context.Context, folderID, deviceID string) error {
	path := "/rest/config/folders/" + url.PathEscape(folderID)
	var existing map[string]any
	if err := c.get(ctx, path, nil, &existing); err != nil {
		return err
	}
	return c.shareWith(ctx, path, existing, []string{deviceID})
}

func (c *Client) shareWith(ctx context.Context, path string, folder map[string]any, devices []string) error {
	current, _ := folder["devices"].([]any)
	have := map[string]bool{}
	for _, d := range current {
		if m, ok := d.(map[string]any); ok {
			id, _ := m["deviceID"].(string)
			have[id] = true
		}
	}
	changed := false
	for _, d := range devices {
		if !have[d] {
			current = append(current, map[string]any{"deviceID": d})
			have[d] = true
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return c.do(ctx, "PATCH", path, nil, map[string]any{"devices": current}, nil)
}

type PendingDevice struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Time    time.Time `json:"time"`
}

// PendingDevices lists devices that tried to connect but aren't added yet.
func (c *Client) PendingDevices(ctx context.Context) ([]PendingDevice, error) {
	var raw map[string]PendingDevice
	if err := c.get(ctx, "/rest/cluster/pending/devices", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]PendingDevice, 0, len(raw))
	for id, d := range raw {
		d.ID = id
		out = append(out, d)
	}
	return out, nil
}

// Wait polls until Syncthing answers, for right after it's been installed.
func (c *Client) Wait(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := c.MyID(ctx)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		if c.APIKey == "" {
			c.APIKey = discoverAPIKey()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
