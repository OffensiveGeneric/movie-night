package setup

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/offensivegeneric/movie-night/internal/config"
	"github.com/offensivegeneric/movie-night/internal/syncplay"
	"github.com/offensivegeneric/movie-night/internal/syncthing"
)

const usage = `Usage:
  movienight setup host [-name N] [-library DIR]... [-room R] [-server HOST:PORT] [-dry-run]
      On the computer with the library. Prints a code for the other person.
  movienight setup join [CODE] [-name N] [-dir DIR] [-dry-run]
      On the other computer, with the host's code.
  movienight setup
      Asks which of the two you are.`

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ", ") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// Main runs "movienight setup ...". It returns the process exit code.
func Main(args []string, in io.Reader, out io.Writer) int {
	p := &prompter{in: bufio.NewReader(in), out: out}
	if len(args) == 0 {
		switch strings.ToLower(p.ask("Is the library on this computer (host), or are you joining someone (join)?", "join")) {
		case "host", "h":
			args = []string{"host"}
		default:
			args = []string{"join"}
		}
	}
	var err error
	switch args[0] {
	case "host":
		err = runHost(args[1:], p, out)
	case "join":
		err = runJoin(args[1:], p, out)
	case "-h", "-help", "--help", "help":
		fmt.Fprintln(out, usage)
		return 0
	default:
		fmt.Fprintln(out, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(out, "\nSetup stopped: %v\n", err)
		p.pause()
		return 1
	}
	p.pause()
	return 0
}

func runHost(args []string, p *prompter, out io.Writer) error {
	fs := flag.NewFlagSet("setup host", flag.ContinueOnError)
	fs.SetOutput(out)
	var libs multi
	name := fs.String("name", "", "your name, as shown in invites")
	fs.Var(&libs, "library", "a library folder to share (repeat for more)")
	room := fs.String("room", "", "Syncplay room (default: keep the current one, or make one)")
	server := fs.String("server", "", "Syncplay server host:port (default: current, or syncplay.pl:8997)")
	dry := fs.Bool("dry-run", false, "show what would change without changing it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sp, _ := syncplay.Read(syncplay.ConfigPath())
	if *name == "" {
		*name = p.ask("Your name", firstNonEmpty(sp.Name, userName()))
	}
	if len(libs) == 0 {
		def := sp.MediaDirs
		if cfg, err := loadConfig(); err == nil && len(cfg.Libraries) > 0 {
			def = nil
			for _, l := range cfg.Libraries {
				def = append(def, l.Path)
			}
		}
		answer := p.ask("Library folders to share, separated by ;", strings.Join(def, "; "))
		for _, d := range strings.Split(answer, ";") {
			if d = strings.TrimSpace(d); d != "" {
				libs = append(libs, d)
			}
		}
	}
	for _, l := range libs {
		if st, err := os.Stat(l); err != nil || !st.IsDir() {
			return fmt.Errorf("%s isn't a folder on this computer", l)
		}
	}
	if *server == "" {
		*server = firstNonEmpty(sp.Server, "syncplay.pl:8997")
	}
	if *room == "" {
		*room = firstNonEmpty(sp.Room, NewRoom())
	}

	env, ctx, err := prepare(out, *dry)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\nSetting up sharing:")
	code, err := env.Host(ctx, HostOptions{Name: *name, Libraries: libs, Server: *server, Room: *room})
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintf(out, "\nDry run: nothing was changed. The code would be:\n\n  %s\n", code)
		return nil
	}
	if _, err := env.InstallSelf(); err != nil {
		return err
	}
	fmt.Fprintf(out, `
All set. Send this code to the person you're watching with:

  %s

They run:  movienight setup join <code>
When their computer asks to connect, Movie Night shows an Accept button.
`, code)
	return nil
}

func runJoin(args []string, p *prompter, out io.Writer) error {
	fs := flag.NewFlagSet("setup join", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("name", "", "your name")
	dir := fs.String("dir", "", "where to put the shared folders (default: your Videos folder)")
	dry := fs.Bool("dry-run", false, "show what would change without changing it")
	// Allow the code before or after the flags.
	var raw string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		raw, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if raw == "" && fs.NArg() > 0 {
		raw = fs.Arg(0)
	}
	if raw == "" {
		raw = p.ask("Paste the code you were sent", "")
	}
	code, err := ParseCode(raw)
	if err != nil {
		return err
	}

	sp, _ := syncplay.Read(syncplay.ConfigPath())
	if *name == "" {
		*name = p.ask("Your name", firstNonEmpty(sp.Name, userName()))
	}
	if *dir == "" {
		home, _ := os.UserHomeDir()
		*dir = p.ask("Where should the shared folders go?", filepath.Join(home, "Videos"))
	}

	env, ctx, err := prepare(out, *dry)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nPairing with %s:\n", code.Name)
	myID, err := env.Join(ctx, code, JoinOptions{Name: *name, Dir: *dir})
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintln(out, "\nDry run: nothing was changed.")
		return nil
	}
	installed, err := env.InstallSelf()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, `
All set. Last step is on %s's side: their Movie Night will show
"%s's computer wants to connect". Once they click Accept, the library
starts copying here.

(If they need it, this computer's Syncthing ID is %s)
`, code.Name, *name, myID)
	if installed != "" {
		fmt.Fprintln(out, `Open "Movie Night" from your desktop to watch.`)
	}
	return nil
}

// prepare installs missing programs and waits for Syncthing's API.
func prepare(out io.Writer, dry bool) (*Env, context.Context, error) {
	ctx := context.Background()
	fmt.Fprintln(out, "Checking programs:")
	paths, err := EnsureDeps(ctx, out, dry)
	if err != nil {
		return nil, nil, err
	}
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, nil, err
	}
	st := syncthing.New("http://127.0.0.1:8384", "")
	if err := st.Wait(ctx, 90*time.Second); err != nil {
		return nil, nil, fmt.Errorf("Syncthing isn't answering (%v). Start it (or log out and back in) and run setup again", err)
	}
	return &Env{Out: out, Syncthing: st, SyncplayIni: syncplay.ConfigPath(), ConfigPath: cfgPath,
		VLC: paths["VLC"], DryRun: dry}, ctx, nil
}

func loadConfig() (*config.Config, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return config.Load(path)
}

type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p *prompter) ask(q, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", q, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", q)
	}
	line, err := p.in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" || errors.Is(err, io.EOF) && line == "" {
		return def
	}
	return line
}

// pause keeps a double-clicked console window open long enough to read.
func (p *prompter) pause() {
	fmt.Fprint(p.out, "\nPress Enter to close.")
	p.in.ReadString('\n')
}

func userName() string {
	if u, err := user.Current(); err == nil {
		n := u.Name
		if n == "" {
			n = u.Username
		}
		if i := strings.LastIndexAny(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		if f := strings.Fields(n); len(f) > 0 {
			return f[0]
		}
	}
	return "Me"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
