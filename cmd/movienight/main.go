// Command movienight serves a local web UI for browsing the media library and
// starting synced playback through Syncplay.
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/offensivegeneric/movie-night/internal/art"
	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/server"
	"github.com/offensivegeneric/movie-night/internal/state"
	"github.com/offensivegeneric/movie-night/internal/syncplay"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
	"github.com/offensivegeneric/movie-night/internal/tmdb"
)

func main() {
	defaultConfig, _ := config.DefaultPath()
	configPath := flag.String("config", defaultConfig, "path to config.json")
	addr := flag.String("addr", "", "listen address (overrides the config)")
	noBrowser := flag.Bool("no-browser", false, "don't open the UI in a browser")
	flag.Parse()

	cfg, err := loadOrDetect(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *addr != "" {
		cfg.Addr = *addr
	}

	ctx := context.Background()
	opt := server.Options{Config: cfg, Role: server.Solo}

	// Syncthing tells us which library is shared and in which direction.
	st := syncthing.New(cfg.Syncthing.URL, cfg.Syncthing.APIKey)
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	folders, err := st.Folders(probe)
	cancel()
	if err != nil {
		log.Printf("Syncthing not available (%v); download status and invites are off", err)
	} else {
		opt.Syncthing, opt.Folders = st, folders
	}

	// The first library that is a Syncthing folder holds the shared
	// .movienight directory: invites and artwork travel through it.
	var shared string
	for _, l := range cfg.Libraries {
		if f, ok := syncthing.FolderFor(folders, l.Path); ok {
			shared = filepath.Join(l.Path, ".movienight")
			opt.Role = server.Host
			if f.Type == "receiveonly" {
				opt.Role = server.Guest
			}
			break
		}
	}

	cacheDir, _ := os.UserCacheDir()
	local := art.NewStore(filepath.Join(cacheDir, "movienight", "art"))
	fetchInto := local
	if shared != "" {
		opt.InvitePath = filepath.Join(shared, "invite.json")
		sharedArt := art.NewStore(filepath.Join(shared, "art"))
		opt.ArtStores = append(opt.ArtStores, sharedArt)
		// The guest's copy of the shared folder is receive-only, so only the
		// host writes art there.
		if opt.Role == server.Host {
			fetchInto = sharedArt
		}
	}
	opt.ArtStores = append(opt.ArtStores, local)
	if cfg.TMDBKey != "" {
		opt.Fetcher = &art.Fetcher{Client: tmdb.New(cfg.TMDBKey), Store: fetchInto, Overrides: cfg.ArtOverrides}
	}

	opt.State = state.Load(filepath.Join(filepath.Dir(*configPath), "state.json"))

	srv := server.New(opt)
	srv.Refresh(ctx)
	go srv.Run(ctx)

	lib := srv.Library()
	episodes := 0
	for _, s := range lib.Shows {
		episodes += len(s.Episodes)
	}
	log.Printf("%s (%s): %d shows (%d episodes), %d movies", cfg.Name, opt.Role, len(lib.Shows), episodes, len(lib.Movies))

	url := "http://" + cfg.Addr
	log.Printf("Movie Night is running at %s", url)
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Handler()))
}

// loadOrDetect reads the config, or on first run builds one from Syncplay's
// settings (its name and media folders) so there's nothing to set up.
func loadOrDetect(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return cfg, err
	}
	sp, err := syncplay.Read(syncplay.ConfigPath())
	if err != nil {
		return nil, err
	}
	if len(sp.MediaDirs) == 0 {
		return nil, errors.New("no config yet and no media folders in Syncplay's settings: copy config.example.json to " + path)
	}
	var libs []config.Library
	for _, dir := range sp.MediaDirs {
		libs = append(libs, config.Library{Name: filepath.Base(dir), Path: dir, Kind: "auto"})
	}
	name := sp.Name
	if name == "" {
		name = "Me"
	}
	log.Printf("first run: created %s from Syncplay's media folders", path)
	return config.New(path, name, libs)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
