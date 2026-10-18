# Setting up Movie Night for two homes

One person is the **host**: the library lives on their computer. The other is the **guest**: they receive a copy. Each runs one command, and Movie Night installs and configures everything else.

## Quick setup (Windows)

Download `movienight.exe` from the [releases page](https://github.com/OffensiveGeneric/movie-night/releases) on both computers.

**Host**, in a terminal next to the file:

```
movienight setup host
```

It asks for your name and the library folders to share (it suggests the ones Syncplay already knows about), then:

1. installs anything missing: Syncthing, Syncplay and VLC, from their official winget packages;
2. shares each library in Syncthing as **send-only**, reusing any share that already exists;
3. points Syncplay at a room on a server (yours if you already have one, otherwise a new private room on `syncplay.pl:8997`) and applies the settings that keep two players in sync;
4. installs Movie Night to `%LOCALAPPDATA%\MovieNight` with a desktop shortcut;
5. prints a **code** starting with `MN1-`. Send it to the guest.

**Guest:**

```
movienight setup join MN1-...
```

The same installs, then it adds the host's computer to Syncthing, creates matching **receive-only** folders (in your Videos folder by default) that download episodes in order, and points Syncplay at the same room. Double-clicking `movienight.exe` on a computer with nothing set up runs the same steps interactively.

**Host again:** open Movie Night. A banner says *"Dad's computer wants to connect"*. Click **Accept** and the library starts copying.

Every step is safe to re-run, and `-dry-run` shows what would change without changing anything. Existing Syncthing folders and devices are only ever extended, never moved or retyped.

## Watching

- **Host:** click **Play** on an episode. The guest gets a **Join** banner.
- **Guest:** click **Join**. Either of you can pause and seek, and the other follows.
- **Stuck?** Click **Restart player**. It reopens the episode and rejoins where the room is.
- **Artwork:** the host can add a TMDB key to `%APPDATA%\movienight\config.json` (`"tmdbKey": "..."`, free from themoviedb.org → Settings → API). Posters reach the guest through Syncthing, so the guest doesn't need a key.

Windows may warn that `movienight.exe` is from an unknown publisher (it isn't code-signed). Click *More info → Run anyway*.

## Doing it by hand

If you'd rather not use `setup`, or you're not on Windows:

1. Install [Syncthing](https://syncthing.net/), [Syncplay](https://syncplay.pl/) and [VLC](https://www.videolan.org/).
2. Pair the two Syncthing devices. The host shares each library folder as **Send Only**; the guest accepts them as **Receive Only** (*File Pull Order: Alphabetic* is nice for series). If the host filters the share with an `.stignore`, include `!/.movienight`.
3. In Syncplay on both computers, use the same server and room, set the media directories to the library folders, and pick VLC.
4. Run `movienight.exe`. It reads Syncplay's settings on first run and offers to fix the rest.
