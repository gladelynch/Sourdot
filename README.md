# Sourdot — A Godot Version Manager

A Godot Engine version manager + project manager. Install/switch/remove
Godot versions, track a project list with per-project engine-version
pinning, and open any project in the correct engine version with one click
— auto-installing it in the background if it's missing.

Written in Go with [Wails v2](https://wails.io) (Go backend + native OS
webview). The frontend is Preact, bundled by esbuild — and still **no
Node.js and no npm**, because esbuild ships a Go API and Preact is vendored
as plain ESM; see "Why no Node.js" below.

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
wails dev -tags webkit2_41      # live-reloading dev build (see note below)
wails build -tags webkit2_41    # production build -> build/bin/sourdot
```

On Windows/macOS, or older Linux distros that still ship `webkit2gtk-4.0`,
drop the `-tags webkit2_41` flag.

`wails build` bundles the frontend itself. A bare `wails dev` does not —
run `make frontend-watch` beside it, or just use `./dev.sh`, which starts
both.

## The dev loop

`./dev.sh` adds the build tag for you and runs the dev server **detached** —
no terminal window to keep open, and nothing tying up the shell you launched
it from. It starts the frontend bundler in watch mode alongside `wails dev`,
so editing `frontend/src` re-bundles and reloads the window; `--stop` takes
both down together. `Sourdot-Dev.desktop` runs the same script, so
double-clicking the launcher (or right-clicking it for *Restart* / *Stop*)
works the same way.

```
./dev.sh              # start detached
./dev.sh --status     # running? where are the logs?
./dev.sh --log        # follow the build output
./dev.sh --restart    # after a change the watcher didn't pick up
./dev.sh --stop
./dev.sh --fg         # foreground instead, Ctrl-C to stop
./dev.sh --build      # production build, then launch the binary
```

Nothing is lost by having no tty: the app's own output already goes to
`sourdot.log` in the config directory (see `internal/applog`), and `dev.sh`
sends the build output `wails dev` writes — compile errors, reload notices —
to `dev.log` beside it, keeping one previous run as `dev.log.1`.

## The frontend

Preact + JSX, bundled to a single `frontend/dist/js/app.js` (~35 KB
minified, Preact included).

State that comes from the backend lives in exactly one place,
`frontend/src/store.js`. Views never fetch; they subscribe to a named
resource (`installed`, `projects`, `defaultVersionID`, `catalog`, …) and
re-render when it changes. Mutations go through `actions.*`, and each one
declares which resources it invalidated — the store refetches those and
notifies every subscriber, whichever view triggered the change.

That indirection is the whole point. Each view used to keep a private copy
of the same data, so installing a version refreshed the Versions page and
left the Projects page's pin menu and the Settings default picker
advertising builds that were no longer on disk. Adding a mutation now means
answering one question — *what does this make stale?* — instead of
remembering which three other places to poke.

Transient, high-frequency backend output (download percentages, checksum
results) deliberately stays out of the store and goes through the
`frontend/src/events.js` pub/sub instead: it belongs to one in-flight
operation, and pushing every progress tick through a store refresh would
re-render the world sixty times a download. `install_complete` is the
exception — it invalidates the store, so an install the *backend* starts on
its own (`OpenProject` auto-installs) still converges the whole UI.

```
frontend/src/       JS/JSX sources. main.jsx is the entry point.
frontend/public/    index.html, css/, assets/ — copied to dist verbatim.
frontend/vendor/    Preact, vendored as plain ESM (see below).
frontend/dist/      Generated. Gitignored, embedded by main.go.
tools/frontendbuild The bundler.
```

## Why no Node.js

Wails only strictly needs an `embed.FS` of static frontend assets, and
Wails injects bound Go methods onto `window.go.main.App.*` in the webview
at runtime by itself — so the frontend calls Go directly with no codegen.

Adding Preact and JSX didn't change that, because neither piece needs a
Node toolchain:

- **esbuild ships a Go API.** `tools/frontendbuild` is an ordinary Go
  program importing `github.com/evanw/esbuild/pkg/api`, so the bundler is
  `go run ./tools/frontendbuild` and its only transitive dependency is
  `golang.org/x/sys`, which the Wails tree already carried. There is no
  esbuild binary to install and no `node_modules`.
- **Preact is vendored, not installed.** Three ESM files in
  `frontend/vendor/preact/`, wired to the `preact`, `preact/hooks` and
  `preact/jsx-runtime` specifiers by esbuild's alias map (see
  `buildOptions` in the bundler). Upgrading means replacing three files.

`wails.json`'s `frontend:install` stays empty; `frontend:build` runs the Go
bundler, so a bare `wails build` and CI are correct without going through
the Makefile.

```
make frontend         # one-shot bundle (minified)
make frontend-watch   # rebuild on save
make clean            # empty frontend/dist
```

`frontend/dist` is generated and gitignored down to a single `.gitkeep`,
which is why `main.go` embeds it as `//go:embed all:frontend/dist` — the
`all:` prefix stops embed from skipping the dot-file and failing the build
on a fresh clone. A forgotten frontend build shows up as an empty window,
not a compile error.

## Project layout

```
internal/core/      UI-agnostic orchestration (VersionManager, ProjectManager) —
                     the reuse boundary a future CLI/TUI would bind to instead of Wails.
internal/godot/      Release discovery (GitHub API) + install (download/extract/verify).
internal/project/    Project folder scanning, project.godot parsing, pin-file resolution.
internal/store/      Settings (JSON) + BoltDB (versions/projects/release cache).
internal/platform/   Per-OS paths and Launch (opens the Godot editor for a project).
frontend/src/        Preact frontend (see "The frontend"); store.js owns all backend state.
frontend/public/     index.html, CSS and assets, copied into the bundle verbatim.
frontend/vendor/     Preact, vendored as plain ESM — no npm.
tools/frontendbuild/ esbuild-in-Go bundler: `go run ./tools/frontendbuild`.
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
