# BgStats Companion

A lightweight Windows tray application that watches your WoW Classic Era SavedVariables and automatically uploads battleground match data to [bgstats.cc](https://bgstats.cc).

## How it works

1. The companion installs the **BgStats** WoW addon into your Classic Era AddOns folder
2. After each battleground, the addon writes match data to `SavedVariables/BgStats.lua`
3. The companion detects the file change and uploads new matches to bgstats.cc
4. Uploaded matches are marked in the SavedVariables file so they are never sent twice

## Requirements

- Windows 10 or later
- [Go 1.21+](https://go.dev/dl/) (to build from source)
- World of Warcraft Classic Era

## Building

```bat
build.bat
```

The build script will:
- Generate `assets/logo.ico` from `assets/logo.png`
- Embed the addon files and icon into the binary
- Output `dist/BgStatsCompanion.exe`

> **Dependencies:** The script uses [`rsrc`](https://github.com/akavel/rsrc) to embed the Windows manifest and icon. It will be installed automatically via `go install` if not found.

## Usage

Run `BgStatsCompanion.exe`. On first launch it will:

1. Copy itself to `%APPDATA%\BgStats Companion\` and add a startup entry
2. Register with bgstats.cc and obtain an API key (no account required)
3. Open the settings window where you point it at your `_classic_era_` folder

After saving settings the addon is installed and the companion starts watching for new matches in the background. It runs silently in the system tray.

### Tray menu

| Action | Description |
|--------|-------------|
| Left-click | Open settings |
| Settings | Open the settings window |
| Open Config Folder | Open `%APPDATA%\BgStats Companion\` in Explorer |
| Open bgstats.cc | Open the website |
| Exit | Quit the companion |

## Configuration

Settings are stored in `%APPDATA%\BgStats Companion\config.json`:

| Field | Description |
|-------|-------------|
| `apiKey` | Auto-generated on first launch |
| `wowClassicDir` | Path to your `_classic_era_` folder |
| `backendUrl` | API endpoint (default: `https://bgstats.cc`) |
| `pollIntervalSeconds` | How often to check for new matches (default: 30) |

## Project structure

| File | Description |
|------|-------------|
| `main.go` | Entry point, first-run setup, autostart |
| `tray.go` | System tray icon and context menu |
| `settings.go` | Settings window UI |
| `watcher.go` | File polling loop |
| `uploader.go` | HTTP client for the bgstats.cc API |
| `lua.go` | SavedVariables parser and match types |
| `installer.go` | WoW directory detection, addon install, autostart registry |
| `ipc.go` | Single-instance enforcement via named Win32 mutex |
| `dialog.go` | Windows folder picker dialog |
| `config.go` | Config load/save |
| `embed.go` | Embedded addon and icon assets |
| `icon.go` | Tray icon loader |
| `activity.go` | In-memory activity log shown in the settings window |

## License

MIT
