package syncplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "[server_data]\r\nhost = syncplay.pl\r\n\r\n[client_settings]\r\nname = Dad\r\nroom = movienight-abc\r\n" +
	"mediasearchdirectories = ['C:\\\\Users\\\\Dad\\\\Videos\\\\Movie Night', 'D:\\\\Movies']\r\n" +
	"sharedplaylistenabled = True\r\nreadyatstart = True\r\n\r\n[gui]\r\nshowosd = True\r\n"

func TestSetCreatesAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "syncplay.ini")
	dirs := []string{`C:\Users\Dad\Videos\Movie Night`, `D:\Dad's Movies`}
	if err := Set(path, "server_data", []Setting{{"host", "syncplay.pl"}, {"port", "8997"}}); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "client_settings", []Setting{{"name", "Dad"}, {"mediasearchdirectories", FormatList(dirs)}}); err != nil {
		t.Fatal(err)
	}
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Dad" || len(s.MediaDirs) != 2 || s.MediaDirs[0] != dirs[0] {
		t.Errorf("round trip: name %q dirs %q", s.Name, s.MediaDirs)
	}
	// Setting again replaces rather than duplicates.
	Set(path, "client_settings", []Setting{{"name", "Pops"}})
	b, _ := os.ReadFile(path)
	if strings.Count(string(b), "name =") != 1 || !strings.Contains(string(b), "name = Pops") {
		t.Errorf("name not replaced in place:\n%s", b)
	}
}

func TestReadAndFix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "syncplay.ini")
	os.WriteFile(path, []byte(sample), 0o644)

	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Dad" || s.Room != "movienight-abc" {
		t.Errorf("name/room = %q/%q", s.Name, s.Room)
	}
	if len(s.MediaDirs) != 2 || s.MediaDirs[0] != `C:\Users\Dad\Videos\Movie Night` || s.MediaDirs[1] != `D:\Movies` {
		t.Errorf("media dirs = %q", s.MediaDirs)
	}
	if got := len(s.Problems()); got != 3 {
		t.Errorf("problems = %d, want 3 (unpauseaction, sharedplaylistenabled, interval)", got)
	}

	if err := Fix(path); err != nil {
		t.Fatal(err)
	}
	s, _ = Read(path)
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("still wrong after Fix: %v", p)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	if !strings.Contains(text, "[gui]\r\nshowosd = True") || !strings.Contains(text, "host = syncplay.pl") {
		t.Errorf("Fix disturbed other sections:\n%s", text)
	}
	if strings.Index(text, "unpauseaction") > strings.Index(text, "[gui]") {
		t.Errorf("new keys landed outside [client_settings]:\n%s", text)
	}
}
