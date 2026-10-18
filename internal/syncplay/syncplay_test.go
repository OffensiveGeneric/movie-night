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
