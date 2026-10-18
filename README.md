# Movie Night

Watch TV and movies together from two homes, in sync, without streaming.

My dad and I used Plex's Watch Together. It buffered, sometimes wouldn't resume after a pause, and needed Plex accounts on both ends. So we switched to the opposite design: each of us keeps a copy of the library, copied ahead of time, and the only thing that crosses the internet during playback is "play / pause / seek to 12:48". Movie Night is the part that makes that feel like one app instead of three.

## What it does

- **Library with artwork.** Scans your folders, works out show / season / episode and title / year from messy release names, and shows posters and episode stills from TMDB.
- **Watch together.** The host clicks Play; a few seconds later the guest's Movie Night shows *"Tyler is starting Star Trek DS9 S01E12 · Battle Lines [Join]"*. One click and both players open the same episode in sync.
- **Download status.** On the receiving side every episode shows *ready*, *downloading* or *queued*, straight from Syncthing, plus how much is left.
- **Continue watching.** "Next: S01E13" for every show you're partway through.
- **Fixes the known Syncplay traps.** Spots the settings that make playback jump back to 00:00 or stall on "not ready", and fixes them in one click. A **Restart player** button rejoins the room if anything gets stuck.
- **One-command setup, apt-style.** `movienight setup host` installs Syncthing, Syncplay and VLC if they're missing (from their official winget packages), shares the library, configures Syncplay, and prints a pairing code. `movienight setup join <code>` does the rest on the other computer, and the host clicks **Accept**. It's safe to re-run and has a `-dry-run`.

## How it works

```
 Host (Tyler)                                          Guest (Dad)
┌────────────────────────┐                     ┌────────────────────────┐
│ Movie Night (web UI)   │                     │ Movie Night (web UI)   │
│   │ Play       ▲ art   │                     │   ▲ invite, art  │ Join│
│   ▼            │       │   play/pause/seek   │   │              ▼     │
│ Syncplay ◄─► VLC ──────┼─── Syncplay server ─┼──── VLC ◄─► Syncplay   │
│                        │                     │                        │
│ D:\TV  (send-only) ────┼───── Syncthing ────►┼──► Movie Night folder  │
│   .movienight/         │  files, invite.json,│     (receive-only)     │
│     invite.json, art/  │  posters            │                        │
└────────────────────────┘                     └────────────────────────┘
```

- **[Syncthing](https://syncthing.net/)** copies the library peer-to-peer ahead of time. It also carries a tiny `.movienight/` folder holding the current invite and the artwork cache, so the guest needs neither a server, an open port nor a TMDB key.
- **[Syncplay](https://syncplay.pl/)** keeps the two players in step and corrects drift.
- **VLC** (or mpv) plays the local file at full quality, so there's nothing to buffer.
- Every item's ID comes from *what it is* (show + season + episode, or title + year), not where its file lives, so an invite or a poster means the same thing on both machines.

## Setup

On the computer with the library:

```
movienight setup host
```

On the other computer, with the code the first one printed:

```
movienight setup join MN1-...
```

Then the host clicks **Accept** in Movie Night. [docs/setup.md](docs/setup.md) has the details, and the manual route for other platforms.

After that, just run `movienight.exe` (or the desktop shortcut). The UI opens at http://127.0.0.1:8737. Settings live in `%APPDATA%\movienight\config.json` (`~/Library/Application Support/movienight/` on macOS, `~/.config/movienight/` on Linux) that you can edit:

| Key | What it's for |
|---|---|
| `name` | How you appear in invites (defaults to your Syncplay name) |
| `libraries` | `[{ "name", "path", "kind": "tv" \| "movies" \| "auto" }]` |
| `exclude` | Folder or file names to skip, e.g. an app installed inside your Movies folder |
| `tmdbKey` | TMDB API key or read access token, for artwork. Only the host needs one. Or set `MOVIENIGHT_TMDB_KEY`. |
| `artOverrides` | Pin titles that search can't match: `{ "Star Trek DS9": "tv:580" }` |
| `syncplay` | Path to Syncplay, if it isn't in the usual place |
| `syncthing` | `{ "url", "apiKey" }`, if not the local defaults (the key is read from Syncthing itself) |

## Development

```
go test ./...
go build -o bin/ ./cmd/movienight
```

Plain Go standard library, plus a framework-free HTML/JS UI embedded in the binary.

```
cmd/movienight      startup: config / autodetect, role detection, wiring
internal/library    folder scan and release-name parsing
internal/server     HTTP API, background refresh, invites
internal/art        artwork cache and TMDB fetcher
internal/tmdb       minimal TMDB client
internal/syncthing  REST client: folders, what's still downloading, pairing
internal/setup      "movienight setup": installs dependencies, pairs the two machines
internal/syncplay   read and repair Syncplay's settings
internal/invite     the invite file
internal/state      watch history
internal/player     launches Syncplay
web/                the UI
```

## Notes

**Security.** The UI listens on loopback only. The API accepts library item IDs, never file paths, and state-changing requests need a custom header that cross-site pages can't send, so a random web page can't make your machine start playing things.

**The 00:00 bug.** Syncplay re-scans its media folders every 30 seconds by default, and with shared playlists on it can re-open the current file when the folder has changed. On the receiving machine Syncthing changes that folder constantly, so playback kept snapping back to 00:00 every 30 seconds. Turning off shared playlists and stretching the re-scan interval fixes it. That's the first thing the settings check looks for.

**Artwork.** This product uses the TMDB API but is not endorsed or certified by TMDB.

## License

MIT
