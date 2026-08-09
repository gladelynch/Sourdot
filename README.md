# Sourdot — A Godot Version Manager

A Godot Engine version manager + project manager. Install/switch/remove
Godot versions, track a project list with per-project engine-version
pinning, and open any project in the correct engine version with one click
— auto-installing it in the background if it's missing.

Written in Go with [Wails v2](https://wails.io) (Go backend + native OS
webview). The frontend is **plain HTML/CSS/JS — no Node.js, no npm, no
bundler** of any kind; see "Why no Node.js" below.

Full design rationale, competitive research, and the milestone plan live in
this repo's issue tracker / project docs; the short version: this exists
because the official Godot Project Manager can't install or switch engine
versions, and every third-party tool that fills that gap is either
CLI-only, missing a project list, or has broken self-update/reliability
bugs. See `docs/` (added as the project matures) for more.

## Requirements

- Go 1.25+
- The [Wails CLI](https://wails.io/docs/gettingstarted/installation):
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Linux only:** GTK3 + WebKitGTK dev headers, and a C compiler (Wails'
  webview bindings are CGO). On Fedora:
  ```
  sudo dnf install -y gtk3-devel webkit2gtk4.1-devel gcc
  ```
  On Debian/Ubuntu: `sudo apt install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential`.
- **Windows:** the WebView2 Runtime (present by default on Win11/most Win10).
- **macOS:** Xcode command line tools.

No Node.js/npm install step is needed on any platform.

## Building

Newer Linux distros (Fedora 41+, Ubuntu 24.04+, recent Arch/Debian testing)
only ship `webkit2gtk-4.1`, not the older `-4.0` pkg-config name Wails
defaults to — pass the `webkit2_41` build tag on those systems:

```
wails dev -tags webkit2_41      # live-reloading dev build
wails build -tags webkit2_41    # production build -> build/bin/sourdot
```

On Windows/macOS, or older Linux distros that still ship `webkit2gtk-4.0`,
drop the `-tags webkit2_41` flag.

## Why no Node.js

Wails only strictly needs an `embed.FS` of static frontend assets — the
`frontend:install`/`frontend:build` fields in `wails.json` (which normally
shell out to `npm install`/`npm run build`) are left empty here. Wails
injects bound Go methods onto `window.go.main.App.*` in the webview at
runtime by itself, so plain `<script>` tags in `frontend/dist/` call Go
directly with zero build step. `wails dev` still watches `frontend/dist`
and reloads the window on save.

## Project layout

```
internal/core/      UI-agnostic orchestration (VersionManager, ProjectManager) —
                     the reuse boundary a future CLI/TUI would bind to instead of Wails.
internal/godot/      Release discovery (GitHub API) + install (download/extract/verify).
internal/project/    Project folder scanning, project.godot parsing, pin-file resolution.
internal/store/      Settings (JSON) + BoltDB (versions/projects/release cache).
internal/platform/   Per-OS paths and Launch (opens the Godot editor for a project).
frontend/dist/       Plain HTML/CSS/JS, embedded as-is — no build step.
```

## Testing

```
go test ./...                              # fast, offline, runs in CI on every push
go test -tags integration ./internal/core/... -v   # hits the live GitHub API and installs a
                                                     # real Godot version -- run locally, not in CI
```

## CI

`.github/workflows/build.yml` builds on Linux/macOS/Windows on every push
and PR (`go vet`, offline `go test`, and a real `wails build` per OS) —
this is the automatable slice of cross-platform verification. It does
**not** replace a human running the app on real Windows/macOS hardware:
confirming the editor actually launches, no elevation/UAC prompt appears
on Windows, and multiple installed versions coexist correctly on macOS
are still outstanding and need someone with access to that hardware.

## License

GNU General Public License v3.0 — see [`LICENSE`](LICENSE).

`SPDX-License-Identifier: GPL-3.0-or-later`

Note that this is a *separate tool*, not engine code: Godot Engine itself
is MIT-licensed, and nothing here is derived from or linked into it.
