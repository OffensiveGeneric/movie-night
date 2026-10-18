package syncthing

import (
	"path/filepath"
	"testing"
)

func TestLocate(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "home", "dad", "Videos")
	share := filepath.Join(root, "Movie Night")
	folders := []Folder{{ID: "movie-night", Path: share, Type: "receiveonly"}}

	// Library is the share itself.
	m, ok := Locate(folders, share)
	if !ok || m.Sub != "" || m.Outer != "" {
		t.Fatalf("exact: %+v %v", m, ok)
	}
	if rel, ok := m.LibraryRel("DS9/e01.mkv"); !ok || rel != "DS9/e01.mkv" {
		t.Errorf("exact LibraryRel = %q %v", rel, ok)
	}

	// Library is a parent of the share (Syncplay pointed at Videos).
	m, ok = Locate(folders, root)
	if !ok || m.Outer != "Movie Night" {
		t.Fatalf("parent: %+v %v", m, ok)
	}
	if rel, _ := m.LibraryRel("DS9/e01.mkv"); rel != "Movie Night/DS9/e01.mkv" {
		t.Errorf("parent LibraryRel = %q", rel)
	}

	// Library is a subfolder of the share.
	m, ok = Locate(folders, filepath.Join(share, "TV"))
	if !ok || m.Sub != "TV" {
		t.Fatalf("child: %+v %v", m, ok)
	}
	if rel, ok := m.LibraryRel("TV/DS9/e01.mkv"); !ok || rel != "DS9/e01.mkv" {
		t.Errorf("child LibraryRel = %q %v", rel, ok)
	}
	if _, ok := m.LibraryRel("Movies/x.mkv"); ok {
		t.Errorf("child LibraryRel accepted a file outside the library")
	}

	// A sibling with a shared name prefix is not inside.
	if _, ok := Locate(folders, filepath.Join(root, "Movie Night - Movies")); ok {
		t.Errorf("'Movie Night - Movies' matched 'Movie Night'")
	}
}
