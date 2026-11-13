# Claude Dashboard for Kindle Paperwhite 5

Turns a jailbroken Kindle Paperwhite 5 into an e-ink display for the time, the date, the battery, your Claude plan usage and the music playing on your PC.

> **Unofficial tool.** This project is not affiliated with or supported by Anthropic. It reads the Claude sign-in token already stored on your machine, renews that token itself and calls an undocumented usage endpoint. This can stop working at any time, and you use it with your account at your own risk. The token never leaves your machine and never appears in logs, the window or HTTP responses.
>
> The HTTP server on port 8377 has no authentication: anyone on the same LAN can read your usage and control your music. Run it only on a home network you trust.

License: [MIT](LICENSE).

- `server/`: `claudedash.exe`, a Windows tray application. It describes the Kindle's widgets on port 8377 of the LAN, runs their buttons, and announces itself on the LAN so the Kindle finds it. `server/cmd/claudedash-kindle` is the Kindle app.
- `kindle/package/`: the files installed on the Kindle next to the app: the launcher, the fonts and a package manifest.

## 1. Run the application on the PC

`claudedash.exe` is a Wails application: the core is written in Go and the window in Svelte. Building it needs Go, Node and `wails3`:

```bash
cd server
./build.sh
```

This generates the bindings, builds the interface and produces `server/bin/claudedash.exe`. Run that file: an icon appears in the system tray and the window opens.

- Click the tray icon to open the window. Hover over it for a quick reading, for example `Claude: week 28%, Fable 6%`.
- Right-click it for `Open Claude Dashboard`, `Refresh usage` and `Quit`.
- Closing the window minimizes the application to the tray, where it keeps serving the Kindle (this can be changed on the Settings page).

The window has four pages:

| Page | Used for |
|---|---|
| Overview | Claude usage, token status, the current track and whether the Kindle is connected |
| Layout | Which widgets the Kindle shows, their size and where they sit on the Kindle's grid |
| Screen | Background picture with zoom and position, panel transparency, front light and warmth, periodic and manual full refresh, polling interval |
| Settings | Port, usage polling interval, start with Windows, minimize to tray on close |
| Logs | The latest 500 log lines, filtered by category |

All settings are saved to `config.json` next to the exe. Changes made on the Layout and Screen pages reach the Kindle within a few seconds, with no cable.

The usage polling interval cannot go below 5 minutes: polling every 2 minutes got HTTP 429 (rate limited) from Claude after about an hour. On a 429, `claudedash` stops asking for 15 minutes, or for as long as Claude says. The last good reading is saved to `usage-cache.json`, so a restart still has numbers to show.

### Widgets

The Kindle screen is a grid of 6 columns by 8 rows. The top right corner (row 1, columns 4 to 6) is kept for the battery, refresh and exit buttons. On the Layout page, each widget has a row of choices: `None` hides it, the others are the sizes it can be drawn at, in columns × rows. Drag a widget in the preview to move it; it snaps to the grid and never overlaps another widget. A hidden widget remembers its spot.

| Widget | Sizes |
|---|---|
| Clock | 3×1, 3×2 (larger text) |
| Claude usage | 6×1, 3×2 (at most four limits) |
| Music | 6×2, 3×2, 6×1 (without the volume row) |

The PC describes every widget as a small tree of components (text, bars, icons, pictures, buttons, a clock and a countdown) and the Kindle only draws them. The clock, the countdowns and the drop to 0% at a reset are worked out on the Kindle, so they stay right while the PC is off; the rest shows its last state, and the corner says `PC offline`. The Kindle keeps the last screen on disk and shows it again after a restart.

### Adding a widget

1. Write a type implementing `widget.Kind` in `server/internal/widget/` (see `clock.go`): a name, a label, a lucide icon name for the window, its sizes, and `Build(size, data)` returning the tree.
2. Add it to `widget.Default` in `catalog.go`. It appears on the Layout page, hidden until chosen.
3. Icons it needs go in `server/internal/widget/icons/` as SVG. Buttons name an action `<kind>.<action>`, served by implementing `widget.Actor`.

Nothing has to change or be copied on the Kindle. The layout rules both sides follow are at the top of `server/internal/tree/layout.go`; `server/internal/widget/testdata/tree_fixtures.json` checks the Kindle and the window against each other.

### About the token

The server reads the token from `%USERPROFILE%\.claude\.credentials.json`, the sign-in file of Claude Code (the desktop app uses the same file for its Code tab). The access token lives for about 8 hours and the desktop app does not renew it regularly, so when it is about to expire `claudedash` **renews it itself** with the refresh token in the file and writes the new pair back:

- Only the token fields and their expiry are changed; every other field is kept as it was.
- The file is written to a temporary file and renamed, so a half-written file is never left behind.
- The token never appears in logs, the tray or HTTP responses.

Known risk: a refresh token can be used only once. If Claude Code (the CLI or the Code tab) holds the old one in memory and uses it later, it is refused and you have to sign in again there. When `claudedash` itself is refused, the tray shows an error asking you to sign in to Claude again.

Neither the usage endpoint nor the renewal flow is officially documented, and both may change.

### Open the firewall

Run this once in PowerShell or CMD **as administrator**:

```bash
netsh advfirewall firewall add rule name="ClaudeDash" dir=in action=allow protocol=TCP localport=8377 profile=private
```

Windows must treat your home Wi-Fi as a Private network. To remove the rule: `netsh advfirewall firewall delete rule name="ClaudeDash"`.

### Finding the PC

Every 2 seconds the PC broadcasts a small UDP announcement on port 8378 with its name and port, and the Kindle connects to the first PC it hears, remembering it for next time. With several PCs running the app, the corner offers a `Use <name>` button to switch. No IP address has to be entered, and the PC's address may change.

### Media control

`claudedash.exe` reads the current Windows media session (what the volume flyout shows when you press a media key), so it works with Spotify, browsers and most players.

| Path | Purpose |
|---|---|
| `GET /screen.json?battery=N` | Every visible widget with its place and tree, icon hashes, and the background, settings and refresh hashes |
| `POST /action/<name>` | Runs a widget button's action, such as `media.next`, and answers like `/screen.json` |
| `GET /icons/<name>.svg` | An icon named by a tree |
| `GET /media/art.png` | Grayscale cover art, fitted to 400 x 400 |
| `GET /background.png?w=W&h=H` | The background picture, cropped and in grayscale |
| `GET /kindle.json` | Display settings: panel transparency, light, warmth, refresh |

Volume is the master volume of the default output device and changes by 5 per tap.

### Background picture

On the Screen page of the window, choose a JPG or PNG picture. The PC crops it to fill the Kindle screen without stretching it and converts it to grayscale. Drag the picture in the preview to choose which part is kept, and use the zoom slider or the mouse wheel (100% to 500%) to show only part of it. The Kindle downloads the result within a few seconds and keeps it, so the background stays when the PC is off.

Each widget sits on its own panel that lightens the picture behind its text; the `Panel transparency` slider sets how much. With a background picture, e-ink updates flash more easily than on a white background.

### Light and refresh

The Screen page also sets the front light and its warmth, in the Paperwhite 5's own levels 0 to 24. Until you move a slider, the Kindle keeps its own setting. `Refresh screen now` asks the Kindle for one flashing repaint of the whole screen, which clears e-ink ghosting.

## 2. Install the app on the Kindle

The Kindle needs a jailbreak with the hotfix that installs FBInk at `/mnt/us/libkh/bin/fbink`.

1. Build the Kindle app:

   ```bash
   cd server
   ./build-kindle.sh
   ```

   This produces `kindle/dist/claudedash/`.
2. Connect the Kindle to the PC with a USB cable.
3. Copy everything in `kindle/dist/claudedash/claudedash/` and `kindle/dist/claudedash/launch.sh` to a `claudedash` folder at the root of the Kindle.
4. Copy `kindle/dist/claudedash/scriptlets/Claude Dashboard.sh` to `documents/` on the Kindle.
5. Unplug the cable. `Claude Dashboard` appears in the Kindle library; tap it to start the dashboard.
6. Tap `✕` in the top right corner to go back to the Kindle home screen. The circular arrow to its left repaints the whole screen once.

The dashboard takes over the screen from the Kindle interface while it runs and gives it back when it exits, even after a crash. It switches Wi-Fi on if it is off, and keeps the Kindle from going to the screensaver. To avoid black flashes, each update repaints only the widgets that changed.

If the Kindle cannot hear the PC's announcements (some routers block broadcasts), create `claudedash/config.json` on the Kindle containing `{"host": "IP of your PC", "port": 8377}`.

Keep the Kindle on a charger when it is used as a dashboard: Wi-Fi and an always-on screen use far more battery than reading.

## 3. Checklist on the Kindle

1. `Claude Dashboard` is in the library with its cover and opens full screen in portrait.
2. With no PC found yet, the screen says it is looking for Claude Dashboard on the network, then shows the widgets once the PC app runs.
3. The top right corner shows the battery, refresh and exit buttons, outside any panel.
4. The clock, Claude usage and music widgets sit where the Layout page shows them, each on its own panel.
5. The music widget shows the title, artist, application and cover art of what is playing on the PC, and every button works.
6. On the PC, move a widget, change a size and hide one: the Kindle follows within a few seconds.
7. Wait 2 minutes: the clock advances on the minute without heavy ghosting.
8. Quit `claudedash.exe`: within about 10 seconds the corner says `PC offline` and the clock keeps running. Start it again: the mark goes away by itself.
9. Restart the dashboard with the PC off: the last screen comes back.
10. Press the power button to sleep and again to wake: the dashboard is back. A tap while it sleeps only wakes it.
11. Tap `✕`: the Kindle home screen comes back.

## When something goes wrong

- `claudedash/claudedash.log` on the Kindle records what the app did: the PC it found, failed requests and slow commands.
- `claudedash.log` next to the exe on the PC: the line `client <IP> back after ... of silence, last seen HH:MM:SS with ...` tells when the Kindle stopped calling and what its last request was.
- If the screen says the PC was found but cannot be reached, the firewall rule above is missing.

## Development

```bash
cd server
go test -race ./...
```

The Kindle app's drawing, touch and network code is tested on the PC; taking over the screen, the light and Wi-Fi can only be tried on a real device.
