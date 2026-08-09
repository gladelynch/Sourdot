# Sourdot — Status

Snapshot of what's built, what's verified, and what's left. See `README.md`
for build/run instructions and `.github/workflows/build.yml` for CI. The
original design/architecture plan (research, framework decision, milestone
breakdown) lives outside this repo in the planning session that produced
it — this file tracks execution against that plan.

## Completed (M0–M5, all committed on `main`)

| Milestone | What shipped | How it was verified |
|---|---|---|
| **M0 — Scaffolding** | Wails v2 app, fully Node-free frontend (`frontend:install`/`frontend:build` empty in `wails.json`, plain HTML/CSS/JS, Go methods called via `window.go.main.App.*`), dark theme, BoltDB + JSON settings wired into app lifecycle. | Launched the built binary, screenshotted a correctly-titled dark-themed window with working nav. Confirmed zero `node_modules`/`package.json` in the tree. |
| **M1 — Version-manager core** | `internal/godot/release`: GitHub Releases client, ETag-cached; asset-filename classifier (`Classify`) covering Godot 3.x/4.x, standard/mono, all three OSes. `internal/godot/install`: progress-tracked download, zip-slip-guarded extraction (with real symlink recreation for macOS `.app` bundles), SHA-512 checksum verification, collision-proof install layout, binary-path resolution. `internal/core.VersionManager` orchestrates the whole pipeline. | 24 fixture cases in `assets_test.go` against real asset filenames pulled from the GitHub API. Two real bugs found and fixed via those fixtures (a regex ambiguity between standard/mono macOS filenames; a missing arch-passthrough case). Real end-to-end integration tests (`-tags integration`) install and run `--version` on both the standard and mono builds of the current stable release, plus a JSON round-trip test. |
| **M2 — Versions view UI** | Browse available releases with changelog preview, install (standard/mono) with a live progress banner driven by Wails events, list/remove installed versions, set a default. | No click-automation tool available in this environment, so the actual risk (does a `Release` struct survive the exact JSON round-trip the frontend performs when handing one back to `InstallVersion`?) was tested directly and passed. Full `wails build` succeeds. |
| **M3 — Project-manager core** | `internal/project`: minimal `project.godot` parser (name, `config_version` → major-version signal, C# detection), pin-file resolution (`.sourdot-version` own format + read-only compat for `.godot-version`/`.tool-versions`). `internal/platform`: per-OS `Launch`. `internal/core.ProjectManager`: full precedence-chain resolution, auto-install-if-missing, then launch. | Verified the plan's flagged CLI-flag risk against a real downloaded binary's `--help` output (`--path` alone runs the *game*; `--editor` is required to open the editor). Real end-to-end test: added a real project, pinned it to an uninstalled version, opened it, confirmed auto-install + a real editor process launched, killed it immediately after confirming startup. |
| **M4 — Projects view UI** | Add via native folder picker, favorite toggle, inline tag editor, per-project pinned-version dropdown, Open/Remove. | `wails build` succeeds. Added `TestFavoritesAndTagsSurviveRestart` — closes and reopens the real BoltDB file (not just in-memory state) to simulate an app restart, confirming favorites/tags persist. Targets the exact "favorites reset on restart" bug a competitor tool has (per the original research). |
| **M5 — Settings + polish** | Default-version picker, GitHub PAT (hot-applies via `VersionManager.UpdateGitHubToken`, no restart needed), data-directory reveal (`xdg-open`/`open`/`explorer` per OS), app version/backend status. Wired up `Project.Missing` detection (recomputed against the filesystem on every `ListProjects` call) and a matching guard in `OpenProject`. Light accessibility pass (aria-labels on icon-only controls, `role="dialog"`/`role="progressbar"`). | `go build`/`vet`/`test` and a full `wails build` clean. |

**M6 (partial) — Cross-platform verification:**
- ✅ Added `.github/workflows/build.yml`: builds on `ubuntu-latest`/`macos-latest`/`windows-latest` on every push and PR (`go vet`, offline `go test`, a real `wails build` per OS).
- ✅ Audited the full codebase for anything admin/elevation-adjacent. Only match: the `os.Symlink` call in `extract.go`, which exists purely to faithfully reconstruct macOS `.app` bundle contents during extraction, not for any version-switching mechanism — documented why it can't trigger a Windows UAC prompt in practice.
- ❌ **Not done, and can't be done from this environment**: a human actually running the built app on real Windows and macOS hardware — confirming the editor launches, no elevation prompt appears, and multiple installed versions coexist correctly in Finder on macOS.

## What's genuinely still open

1. **Real M6 sign-off.** Needs someone with access to physical (or VM) Windows and macOS machines. CI now builds successfully on both, but nobody has clicked through the running app on either yet. The `darwin`/`windows` code in `internal/platform/launch_*.go` was cross-compiled and reviewed against documented behavior, not runtime-tested.
2. **Visual confirmation of the M2–M5 UI.** The dev machine's screen locked (idle timeout) partway through this work; everything past M1 was verified through Go-side tests and code review rather than an eyeballed screenshot. Worth a quick manual look: `make build && ./build/bin/sourdot`.
3. **v1.x backlog** (deliberately deferred, not started): export template management, addon/Asset-Library awareness, disk-usage cleanup dashboard, nightly/dev channel support, custom mirror/offline support, a CLI/TUI surface reusing `internal/core`, code signing/notarization, auto-update, system tray, recursive multi-project folder scanning.
4. **Distribution.** The app is unsigned and has no installer/packaging step beyond `wails build`'s default output. Fine for local dev; blocks any kind of public distribution.
5. **Published, but not released.** `main` is public at https://github.com/gladelynch/Sourdot under GPL-3.0-or-later, with CI green on Linux/macOS/Windows. There are no tags and no downloadable artifacts yet — see `DISTRIBUTION-PLAN.md`.

## Quick pointers

- Run it: `make build && ./build/bin/sourdot` (or `make dev` for the live-reload loop).
- Run the fast test suite: `go test ./...`.
- Run the real end-to-end tests (hits the live GitHub API, downloads a real Godot release): `go test -tags integration ./internal/core/... -v`.
- Critical files if picking this back up: `internal/core/versionmanager.go` and `internal/core/projectmanager.go` (the two orchestrators), `internal/godot/release/assets.go` (the asset-naming pattern table — the most likely thing to need a new row when a future Godot release changes naming conventions), `app.go` (the full Wails-bound API surface).
