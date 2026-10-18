// Command movienight serves a local web UI for browsing the media library and
// starting synced playback through Syncplay.
package main

import (
	"flag"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/library"
	"github.com/offensivegeneric/movie-night/internal/server"
)

func main() {
	defaultConfig, _ := config.DefaultPath()
	configPath := flag.String("config", defaultConfig, "path to config.json")
	addr := flag.String("addr", "", "listen address (overrides the config)")
	noBrowser := flag.Bool("no-browser", false, "don't open the UI in a browser")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *addr != "" {
		cfg.Addr = *addr
	}

	lib, err := library.Scan(cfg.Libraries, cfg.Exclude)
	if err != nil {
		log.Fatal(err)
	}
	episodes := 0
	for _, s := range lib.Shows {
		episodes += len(s.Episodes)
	}
	log.Printf("library: %d shows (%d episodes), %d movies", len(lib.Shows), episodes, len(lib.Movies))

	url := "http://" + cfg.Addr
	log.Printf("Movie Night is running at %s", url)
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.ListenAndServe(cfg.Addr, server.New(cfg, lib).Handler()))
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
