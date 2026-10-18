package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/syncplay"
	"github.com/offensivegeneric/movie-night/internal/syncthing/stfake"
)

func TestCodeRoundTrip(t *testing.T) {
	c := Code{Device: "AAAA-BBBB", Name: "Tyler", Server: "syncplay.pl:8997", Room: "movienight-x",
		Folders: []CodeFolder{{"movie-night", "Movie Night"}, {"movie-night-movies", "Movie Night - Movies"}}}
	got, err := ParseCode(c.String())
	if err != nil || got.Device != c.Device || len(got.Folders) != 2 || got.Folders[1].Label != "Movie Night - Movies" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if _, err := ParseCode("hello"); err == nil {
		t.Errorf("accepted a non-code")
	}
	if _, err := ParseCode(c.String()[:20]); err == nil {
		t.Errorf("accepted a truncated code")
	}
}

func newEnv(t *testing.T, fake *stfake.Fake) *Env {
	dir := t.TempDir()
	return &Env{Out: &bytes.Buffer{}, Syncthing: fake.Client(), SyncplayIni: filepath.Join(dir, "syncplay.ini"),
		ConfigPath: filepath.Join(dir, "config.json"), VLC: `C:\VLC\vlc.exe`}
}

func TestHostThenJoin(t *testing.T) {
	ctx := context.Background()

	// Host: TV is already shared (reused as is); Movies gets a new folder.
	host := stfake.New("HOST-ID")
	defer host.Close()
	tv, movies := t.TempDir(), t.TempDir()
	host.AddFolder("movie-night", "Movie Night", tv, "sendonly")
	henv := newEnv(t, host)
	code, err := henv.Host(ctx, HostOptions{Name: "Tyler", Libraries: []string{tv, movies}, Server: "syncplay.pl:8997", Room: "movienight-abc"})
	if err != nil {
		t.Fatal(err)
	}
	if code.Device != "HOST-ID" || len(code.Folders) != 2 || code.Folders[0].ID != "movie-night" {
		t.Fatalf("code = %+v", code)
	}
	newID := code.Folders[1].ID
	if f := host.Folders[newID]; f == nil || f["type"] != "sendonly" || f["path"] != movies || f["rescanIntervalS"] == nil {
		t.Fatalf("new host folder = %+v (want send-only, built from defaults)", f)
	}
	sp, _ := syncplay.Read(henv.SyncplayIni)
	if sp.Room != "movienight-abc" || sp.Server != "syncplay.pl:8997" || len(sp.MediaDirs) != 2 || len(sp.Problems()) != 0 {
		t.Errorf("host syncplay = %+v, problems %v", sp, sp.Problems())
	}

	// Guest joins from the code. One folder already exists from an
	// earlier, manual setup: its path must be kept.
	guest := stfake.New("GUEST-ID")
	defer guest.Close()
	videos := t.TempDir()
	oldTV := filepath.Join(videos, "Old place")
	guest.AddFolder("movie-night", "Movie Night", oldTV, "receiveonly")
	genv := newEnv(t, guest)
	parsed, err := ParseCode(code.String())
	if err != nil {
		t.Fatal(err)
	}
	myID, err := genv.Join(ctx, parsed, JoinOptions{Name: "Dad", Dir: videos})
	if err != nil {
		t.Fatal(err)
	}
	if myID != "GUEST-ID" || guest.Devices["HOST-ID"] == nil {
		t.Fatalf("host device not added: %+v", guest.Devices)
	}
	if got := guest.Folders["movie-night"]["path"]; got != oldTV {
		t.Errorf("existing guest folder moved to %v", got)
	}
	if d := guest.FolderDevices("movie-night"); len(d) != 1 || d[0] != "HOST-ID" {
		t.Errorf("existing folder not shared with host: %v", d)
	}
	nf := guest.Folders[newID]
	wantPath := filepath.Join(videos, "Movie Night - "+filepath.Base(movies))
	if nf == nil || nf["type"] != "receiveonly" || nf["order"] != "alphabetic" || nf["path"] != wantPath {
		t.Fatalf("new guest folder = %+v, want receive-only, alphabetic, at %s", nf, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("guest folder directory not created: %v", err)
	}
	gsp, _ := syncplay.Read(genv.SyncplayIni)
	if gsp.Name != "Dad" || gsp.Room != "movienight-abc" || len(gsp.MediaDirs) != 2 || gsp.MediaDirs[0] != oldTV {
		t.Errorf("guest syncplay = %+v", gsp)
	}
	cfg, err := config.Load(genv.ConfigPath)
	if err != nil || cfg.Name != "Dad" || len(cfg.Libraries) != 2 {
		t.Errorf("guest config = %+v %v", cfg, err)
	}

	// The host can't accidentally pair with itself.
	if _, err := henv.Join(ctx, parsed, JoinOptions{Name: "Tyler", Dir: videos}); err == nil {
		t.Errorf("joining your own code should fail")
	}

	// Running join again changes nothing and doesn't fail.
	if _, err := genv.Join(ctx, parsed, JoinOptions{Name: "Dad", Dir: videos}); err != nil {
		t.Errorf("second join: %v", err)
	}
	if d := guest.FolderDevices(newID); len(d) != 1 {
		t.Errorf("second join duplicated devices: %v", d)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	fake := stfake.New("HOST-ID")
	defer fake.Close()
	env := newEnv(t, fake)
	env.DryRun = true
	lib := t.TempDir()
	if _, err := env.Host(context.Background(), HostOptions{Name: "T", Libraries: []string{lib}, Room: "r"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Folders) != 0 {
		t.Errorf("dry run created folders: %v", fake.Folders)
	}
	if _, err := os.Stat(env.SyncplayIni); err == nil {
		t.Errorf("dry run wrote Syncplay settings")
	}
	if !strings.Contains(env.Out.(*bytes.Buffer).String(), "would share") {
		t.Errorf("dry run didn't describe the change:\n%s", env.Out)
	}
}
