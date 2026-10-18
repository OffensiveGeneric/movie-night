# Setting up Movie Night for two homes

One person is the **host**: the library lives on their computer. The other is the **guest**: they receive a copy. Both install the same four things.

## 1. Install (both)

On Windows, in a terminal:

```
winget install --id Syncthing.Syncthing   # or BillStewart.SyncthingWindowsSetup to start at login
winget install --id Syncplay.Syncplay
winget install --id VideoLAN.VLC
```

Then download `movienight.exe` from the [releases page](https://github.com/OffensiveGeneric/movie-night/releases).

## 2. Share the library with Syncthing

1. Each person opens Syncthing (http://127.0.0.1:8384) and sends their **device ID** to the other. Each adds the other's device.
2. **Host:** add a folder for the library (e.g. `D:\TV`), set the folder type to **Send Only**, and share it with the guest.
3. **Guest:** accept the folder (e.g. into `Videos\Movie Night`) and set the folder type to **Receive Only**.
4. Optional, guest: set *File Pull Order* to **Alphabetic** so episodes arrive in order.

The first library that is a Syncthing folder also carries Movie Night's `.movienight/` folder (invites and artwork). If the host filters the share with an `.stignore`, include `!/.movienight`.

## 3. Point Syncplay at the same room (both)

Open Syncplay once and set:

- **Server**: e.g. `syncplay.pl:8997` (public), or your own
- **Room**: the same unusual room name on both sides
- **Media directories**: the library folders on this computer
- **Player**: VLC

Movie Night offers to fix the remaining settings itself the first time it runs.

## 4. Run Movie Night (both)

Double-click `movienight.exe`. A browser tab opens at http://127.0.0.1:8737. Keep the console window open while you watch.

- **Host:** add a TMDB key to `%APPDATA%\movienight\config.json` (`"tmdbKey": "..."`) for posters. Get one free at themoviedb.org → Settings → API. The guest doesn't need one; artwork travels through Syncthing.
- **Movie night:** the host clicks **Play** on an episode. The guest gets a **Join** banner and clicks it. Either of you can pause and seek, and the other follows.
- **Stuck?** Click **Restart player**. It reopens the episode and rejoins where the room is.

Windows may warn that `movienight.exe` is from an unknown publisher (it isn't code-signed). Click *More info → Run anyway*.
