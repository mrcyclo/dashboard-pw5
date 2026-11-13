# Claude Dashboard for Kindle Paperwhite 5

Turns a jailbroken Kindle Paperwhite 5 running KOReader into an e-ink display for the time, the date, the battery, your Claude plan usage and the music playing on your PC.

> **Unofficial tool.** This project is not affiliated with or supported by Anthropic. It reads the Claude sign-in token already stored on your machine, renews that token itself and calls an undocumented usage endpoint. This can stop working at any time, and you use it with your account at your own risk. The token never leaves your machine and never appears in logs, the window or HTTP responses.
>
> The HTTP server on port 8377 has no authentication: anyone on the same LAN can read your usage and control your music. Run it only on a home network you trust.

License: [MIT](LICENSE).

- `server/`: `claudedash.exe`, a Windows tray application. It serves Claude usage and the current track on port 8377 of the LAN and accepts media commands.
- `claudedash.koplugin/`: the KOReader plugin. It draws the widgets the PC describes (clock, Claude usage, music), asks the PC for them every 5 seconds, and has touch controls.

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
| Overview | Claude usage, token status, the current track, whether the Kindle is connected, and the address to enter on the Kindle |
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
| Clock | 3×1, 6×1 |
| Claude usage | 6×2, 3×3 (at most four limits) |
| Music | 6×2, 6×1 (without the volume row) |

The PC describes every widget as a small tree of components (text, bars, icons, pictures, buttons, a clock and a countdown) and the Kindle only draws them. The clock, the countdowns and the drop to 0% at a reset are worked out on the Kindle, so they stay right while the PC is off; the rest shows its last state, and the corner says `PC offline`. The Kindle keeps the last screen on disk and shows it again after a restart.

### Adding a widget

1. Write a type implementing `widget.Kind` in `server/internal/widget/` (see `clock.go`): a name, a label, a lucide icon name for the window, its sizes, and `Build(size, data)` returning the tree.
2. Add it to `widget.Default` in `catalog.go`. It appears on the Layout page, hidden until chosen.
3. Icons it needs go in `server/internal/widget/icons/` as SVG. Buttons name an action `<kind>.<action>`, served by implementing `widget.Actor`.

No change to the plugin is needed, and nothing has to be copied to the Kindle. The layout rules both sides follow are at the top of `claudedash.koplugin/cd_tree.lua`; `server/internal/widget/testdata/tree_fixtures.json` checks the Kindle and the window against each other.

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

### Fixed IP

The Kindle reaches the PC by IP address, so give the PC a DHCP reservation on your router or a static IP in Windows.

### Media control

`claudedash.exe` reads the current Windows media session (what the volume flyout shows when you press a media key), so it works with Spotify, browsers and most players.

| Path | Purpose |
|---|---|
| `GET /screen.json?battery=N` | Every visible widget with its place and tree, icon hashes, and the background, settings and refresh hashes |
| `POST /action/<name>` | Runs a widget button's action, such as `media.next`, and answers like `/screen.json` |
| `GET /icons/<name>.svg` | An icon named by a tree |
| `GET /media.json` | Title, artist, application, playing or paused, volume, cover art hash |
| `GET /media/art.png` | Grayscale cover art, fitted to 400 x 400 |
| `POST /media/<command>` | `play-pause`, `next`, `previous`, `volume-up`, `volume-down` |

Volume is the master volume of the default output device and changes by 5 per tap.

### Background picture

On the Screen page of the window, choose a JPG or PNG picture. The PC crops it to fill the Kindle screen without stretching it and converts it to grayscale. Drag the picture in the preview to choose which part is kept, and use the zoom slider or the mouse wheel (100% to 500%) to show only part of it. The Kindle downloads the result within a few seconds and keeps it, so the background stays when the PC is off.

Each widget sits on its own panel that lightens the picture behind its text; the `Panel transparency` slider sets how much. With a background picture, e-ink updates flash more easily than on a white background.

### Light and refresh

The Screen page also sets the front light and its warmth, in the Paperwhite 5's own levels 0 to 24. Until you move a slider, the Kindle keeps its own setting. `Refresh screen now` asks the Kindle for one flashing repaint of the whole screen, which clears e-ink ghosting.

## 2. Install the plugin on the Kindle

1. Create `claudedash.koplugin/cd_config_local.lua` containing `return { host = "IP of your PC" }`, using the address shown on the Overview page (add `port = ...` if you changed the port). This file overrides the sample values in `cd_config.lua` and is not tracked by git.
2. Connect the Kindle to the PC with a USB cable.
3. Copy the whole `claudedash.koplugin` folder to `koreader/plugins/` on the Kindle, including `icons/`. When updating from a version before widgets, delete the old `cd_layout.lua` there. The `spec/` folder is only for tests and can be left out.
4. Unplug the cable and restart KOReader.
5. In KOReader: top menu, tools (the wrench icon), `Claude Dashboard`. If it is missing, check that the plugin is enabled in the plugin manager.
6. Tap `✕` in the top right corner to leave the dashboard. The circular arrow to its left repaints the whole screen once.

To avoid black flashes, each update repaints only the widgets that changed.

Keep the Kindle on a charger when it is used as a dashboard: Wi-Fi and an always-on screen use far more battery than reading.

## 3. Checklist on the Kindle

1. `Claude Dashboard` is in the KOReader menu and opens full screen in portrait.
2. The top right corner shows the battery, refresh and exit buttons, outside any panel.
3. The clock, Claude usage and music widgets sit where the Layout page shows them, each on its own panel.
4. `Claude · weekly limits` has the rows `All models` and `Fable only`, matching the tray tooltip on the PC.
5. The music widget shows the title, artist, application and cover art of what is playing on the PC.
6. Every button works: previous, play or pause, next, volume down and up. Play or pause and the volume number change at once.
7. On the PC, move a widget, change a size and hide one: the Kindle follows within a few seconds.
8. Wait 2 minutes: the clock advances on the minute without heavy ghosting.
9. Wait past the Kindle's sleep timeout: the screen does not switch to the screensaver.
10. Quit `claudedash.exe`: within about 10 seconds the corner says `PC offline`, the clock and countdowns keep running, and the interface does not stutter. Start it again: the mark goes away by itself.
11. Close and reopen the dashboard with the PC off: the last screen comes back.
12. Tap `✕`: the dashboard closes and the Kindle goes back to sleeping normally.
13. If KOReader reports an error, look at `koreader/crash.log`.

## When the dashboard seems frozen

The cause found so far (2026-11-11): the Kindle was not frozen, it had **gone to sleep**. KOReader's AutoSuspend plugin suspends the device 15 minutes after the last touch. Because the dashboard blocks the Kindle screensaver, the device slept with the last frame still on screen and the touch panel off, which looks exactly like a freeze. In that state the power button did not wake it either: KOReader wakes only when the Kindle reports leaving the screensaver, and with the screensaver blocked that signal never came. The only way out was holding the power button for about 40 seconds to reboot. The plugin now pauses AutoSuspend while the dashboard is open, and if the device still goes to sleep for another reason it releases the screensaver block so the sleep and wake cycle runs normally.

How to recognise it in `claudedash.log` next to the exe: lines `back after ... of silence` with gaps that keep growing (a few minutes, 16 minutes, 30 minutes, an hour), each followed by only one or two requests. That is a sleeping device waking up now and then.

Every network call of the plugin runs in a subprocess with a deadline (`cd_net.lua`, `cd_async.lua`), so a PC that is off or a dropped network costs one update and does not block the touch screen. If a freeze still happens, there are two places to look:

- `claudedash.log` on the PC: the line `client <IP> back after ... of silence, last seen HH:MM:SS with ...` tells when the Kindle stopped calling and what its last request was.
- `koreader/crash.log` on the Kindle: the line `claudedash: UI thread was blocked for about N seconds` is written when a periodic step runs 3 seconds or more late.

## Development

```bash
cd server
go test -race -count=1 ./...
```

`-count=1` is required: Go cannot see that `.lua` files outside the module changed and would return cached results.

This also runs the plugin's Lua specs (`claudedash.koplugin/spec/`) in an embedded Lua VM and checks the syntax of every plugin `.lua` file. The plugin's interface can only be tried on a real device.
