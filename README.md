# Movie Night

A small app for watching TV and movies together from two homes, in sync, without streaming.

My dad and I used Plex's Watch Together. It buffered, sometimes wouldn't resume after a pause, and needed Plex accounts on both ends. So we switched to the opposite design: each of us keeps a copy of the library, copied ahead of time, and the only thing that crosses the internet during playback is "play / pause / seek to 12:48". Movie Night is the part that makes that feel like one app instead of three.

## How it works

```
 Tyler's PC                                          Dad's PC
┌──────────────────────┐                     ┌──────────────────────┐
│ Movie Night (web UI) │                     │ Movie Night (web UI) │
│        │ launch      │                     │      launch │        │
│        ▼             │   play/pause/seek   │             ▼        │
│ Syncplay ◄─► VLC ────┼──── Syncplay server ┼──── VLC ◄─► Syncplay │
│                      │                     │                      │
│ D:\TV, D:\Movies ────┼──── Syncthing ─────►┼──► Movie Night folder│
└──────────────────────┘ (copies files ahead)└──────────────────────┘
```

- **[Syncthing](https://syncthing.net/)** copies the library peer-to-peer ahead of time: send-only from the library side, receive-only on the other.
- **[Syncplay](https://syncplay.pl/)** keeps the two players in step and corrects drift.
- **VLC** (or mpv) plays the local file at full quality, with nothing to buffer.
- **Movie Night** scans the library, shows it as a browsable catalogue, and opens the chosen episode in Syncplay with one click.

## Status

Early. What works now:

- Scans TV and movie folders and works out show / season / episode and title / year from messy release names.
- Web UI for browsing shows, seasons and movies, with search.
- **Play** opens the item in Syncplay, closing any earlier session first.

Roadmap:

- [ ] Posters and backdrops from TMDB; episode thumbnails extracted from the files
- [ ] Download status per episode on the receiving side (ready / downloading % / queued), from Syncthing's REST API
- [ ] **Watch together** invite: one side picks an episode, the other gets a "Join" banner
- [ ] Continue watching / next episode
- [ ] "Fix it" button: restart Syncplay and rejoin at the room's position
- [ ] Packaged builds for Windows and macOS

## Running it

Requires Go 1.22+ and Syncplay with VLC or mpv.

```
go build -o bin/ ./cmd/movienight
```

Copy `config.example.json` to your config directory (`%APPDATA%\movienight\config.json` on Windows, `~/Library/Application Support/movienight/config.json` on macOS, `~/.config/movienight/config.json` on Linux), set your library paths, then run `bin/movienight`. The UI opens at http://127.0.0.1:8737.

## Notes

The UI listens on loopback only. The API accepts library item IDs, never file paths, and state-changing requests need a custom header that cross-site pages can't send, so a random web page can't make your machine start playing things.

Lesson learned while building it: Syncplay re-scans its media folders every 30 seconds by default, and with shared playlists on it can re-open the current file when the folder changes. On the receiving machine, Syncthing changes that folder constantly, so playback kept snapping back to 00:00. Setting `sharedplaylistenabled = False` and a long `foldersearchdoublecheckinterval` fixed it.

## License

MIT
