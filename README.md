# Quintessence Launcher

The launcher for **Quintessence**, a private *New World* server project. It starts your own copy of the original game client against a server you choose, with the name you choose, and gets out of the way.

This is a fork of [Azoth Launcher](https://github.com/KyzaBOY/azoth-launcher) by tempesTT (MIT). All the reverse engineering of the client (the version check, the isolated game copy and the four in-memory patches) is theirs. What this fork changes:

- **No Discord login and no account website.** You set a **player name** in Settings, and the launcher passes it into the game as the login persona.
- **The server can be set in Settings** (`host:port`, for example `127.0.0.1:6000` for a server running on your own PC). A build can still have a default server baked in.
- **No logo pak.** The launcher adds no files to your New World install.
- Renamed throughout: `%LOCALAPPDATA%\QuintessenceLauncher`, the `QuintessenceClient` copy folder.

> **Quintessence is a fan project. It is not affiliated with, endorsed by or connected to Amazon or Amazon Games, or with Project Azoth.** *New World* is a trademark of Amazon.com, Inc. or its affiliates. You need your own copy of the game, and nothing from the game is included in this repository.

## What it does

1. **Finds your game.** You pick the New World folder in Settings (or press *Search*, which looks through your Steam libraries and local drives).
2. **Checks the version.** Only build `1.400.6031.6004151` is supported. The launcher computes the sha256 of `NewWorld.exe` and refuses anything else.
3. **Plays from a copy.** Your install is never run or changed. On first Play the launcher builds an isolated copy in `%LOCALAPPDATA%\QuintessenceLauncher\client` (the `Bin64` folder plus a few small config files, about 270 MB). The large `assets` folder is an NTFS junction back to your install, so it takes no extra space. You can move the copy in Settings.
4. **Starts the game suspended, patches it in memory, resumes it.** If any patch site doesn't hold the expected bytes, the process is killed instead of resumed.

## The four in-memory patches

All four are written into the running game process only. No file on disk is changed.

| # | What it changes | Why |
|---|---|---|
| 1 | **DTLS trust.** One conditional jump in the game's secure-socket initialization becomes an unconditional jump. | So the client accepts a server with a self-signed certificate. |
| 2 | **Menu ping over ICMP.** One pointer in the region pinger's table switches from `https` to `icmp`. | The menu pinger would otherwise talk HTTPS to an Amazon endpoint. |
| 3 | **Ping host.** The pinger reads its target host from a string the launcher wrote. | So the menu ping measures your server. |
| 4 | **Persona.** The login code reads the player name from a string the launcher wrote. | So the server learns the player name instead of the Windows user name. |

Every patch site, with its original and new bytes, is in `internal/client/client.go` and `internal/client/redirect.go`.

**Security note:** patch 1 turns off the client's check of the server certificate. The client will talk to whatever answers at the server address, so only connect to servers you trust, on networks you trust. There is no password: the player name is sent as-is, so a server that needs real accounts has to add its own authentication.

## What is stored

In `%LOCALAPPDATA%\QuintessenceLauncher`:

| File | Content |
|---|---|
| `config.json` | `{"gameDir": "...", "copyDir": "...", "playerName": "...", "server": "host:port"}` |
| `client\` | The isolated game copy, unless you picked another folder for it. |

## Requirements

- Windows 10 or 11, 64-bit, with the WebView2 runtime.
- Your own copy of *New World* at build `1.400.6031.6004151` (sha256 of `NewWorld.exe`: `8654f01d324636d9f74f1c793b0cc4a417c3c5fa9847d9913c358ca29e0fdc8e`).
- A game server that speaks this client's protocol.

## Build

You need Go 1.26 or newer and the [Wails v2](https://wails.io) CLI:

```
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
```

That gives you `build\bin\QuintessenceLauncher.exe`. With no default server baked in, players enter one in Settings. To bake one in:

```
wails build -ldflags "-X main.gatewayHost=your.server.host -X main.gatewayPort=6000"
```

Or copy `release.env.example` to `release.env`, fill it in and run `powershell -ExecutionPolicy Bypass -File scripts\release.ps1`. That also writes a `.sha256` file next to the exe.

**Tests.** `go test ./...` on Windows. If you set `NW_CLIENT_BIN` to a `Bin64` folder of the supported build, one extra test also checks the patch sites against the real `NewWorld.exe`.

**The screen without Wails.** `frontend\index.html` also opens in a plain browser with sample data (`frontend\js\bridge.js`): `?name=Alice` starts with a name set, `?screen=settings` opens Settings, `?search=none|one|many` picks what Search finds.

## Layout

| Path | What is there |
|---|---|
| `main.go`, `app.go`, `flow.go` | Wails app, the methods the screen can call, and the Play pipeline. |
| `internal/client` | The game side: version check, isolated copy, install search, launching and the in-memory patches. |
| `internal/config` | `config.json`. |
| `frontend/` | The screen: plain HTML, CSS and JavaScript, with its fonts. |
| `scripts/release.ps1` | Release build. |

## License

MIT, see [LICENSE](LICENSE). The bundled fonts (IM FELL DW Pica, Archivo, Cinzel Decorative) are under the SIL Open Font License; their license texts are in `frontend/fonts`.
