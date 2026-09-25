# Installers + Self-Update for Sourdot

## Context

Sourdot is currently a dev-only portable build: `wails build` drops a binary in `build/bin/`, `dev.sh` and `Sourdot-Dev.desktop` are a dev-only launcher, there are no tags, and `AppVersion` is a hand-bumped `const` in `app.go:124`. CI compiles on three OSes but publishes nothing. STATUS.md lists distribution and auto-update as the blockers to public release.

Goal: real installers for **Linux + Windows**, published from a **public GitHub repo** on a tag push, with the app **checking for updates on launch, prompting, and self-replacing** — payloads authenticated by an **Ed25519 signature** whose private key lives in GitHub Actions so releasing stays a one-command, set-and-forget operation. macOS is explicitly out of scope (no Apple Developer account); keep its CI compile check only.

## Verified constraints (checked against Wails v2.13.0 source, not assumed)

- `wails build -nsis -installscope user` emits **both** `-DWAILS_INSTALL_SCOPE=user` and `-DREQUEST_EXECUTION_LEVEL=user` (`pkg/commands/build/nsis_installer.go:103-107`). **No `project.nsi` edits needed for per-user install** — this is what preserves the project's no-elevation stance and makes UAC-free self-update possible.
- `wails build -ldflags "..."` passes through verbatim (`pkg/commands/build/base.go:248-251`). Pass one combined string.
- `wails_tools.nsh` is regenerated on every `-nsis` build — never edit it. `project.nsi` is written only if missing, so it's ours.
- `wails.json` is **not** templated; `info.productVersion` must be numeric `X.Y.Z` (project.nsi appends `.0` for `VIProductVersion`). CI must patch it with `jq`.
- The NSIS `wails.files` macro installs only the single `.exe` — so a Windows install is a one-file payload, and a one-file swap is a complete update.

## Design decisions

**Linux artifact: AppImage** (plus a non-self-updating `.tar.gz` secondary). `$APPIMAGE` gives the exact file path to replace with zero guessing — decisive, since self-update is the whole feature. `.deb`/`.rpm` are rejected: updating them needs root and self-replacing a package-owned file desyncs the package DB.

**Do not bundle libwebkit2gtk** into the AppImage. It's a known-broken path (helper processes resolve via hardcoded `/usr/lib/.../webkit2gtk-4.x/`; `linuxdeploy-plugin-gtk` doesn't fix it). Depend on the host's `libwebkit2gtk-4.1` + `libgtk-3`, and skip `linuxdeploy` entirely — a 6-line `AppRun` + `appimagetool` is enough.

**One Linux build, `-tags webkit2_41` only.** Ubuntu 22.04+/Debian 12+/Fedora 38+/Arch all ship webkit2gtk-4.1. A second 4.0 build would double CI *and* force the running binary to know its own flavor to pick an update asset.

**Build Linux on `ubuntu-22.04`, not `ubuntu-latest`.** Wails is CGO; the binary's glibc floor equals the builder's. Building on 24.04 silently produces something that won't run on Debian 12 / Ubuntu 22.04.

**Update feed = a static signed manifest, not the GitHub API.** Fetch `https://github.com/gladelynch/sourdot/releases/latest/download/manifest.json` + `.sig`. This is the releases CDN, not `api.github.com`, so it consumes **zero** of the 60/hr quota shared with the Godot catalog, needs no PAT, and `/latest/` excludes prereleases for free (giving a safe dress-rehearsal channel).

**Do not reuse `release.Client`.** Its `tagPattern` requires a `-label` suffix and so rejects `v1.2.3` outright, `minSupportedMajor = 3` drops `v1.x`, it paginates 8 pages through Godot asset regexes, and it carries the user's PAT. A ~70-line `feed.go` is the right call. Similarly `install.VerifyChecksum` is a bad fit (SHA-512 from a network `SHA512-SUMS.txt`, with a "skipped" tri-state the updater must never accept) — hash SHA-256 while streaming instead.

**Do reuse:** `install.Download` (progress throttling + guaranteed final call), `core.Event`/`EventSink`, `store.LoadSettings`/`SaveSettings`, `.progress-bar`/`.progress-bar-fill` CSS (`frontend/dist/css/main.css:174,181`).

---

## Milestone 1 — Version plumbing (0.5d)

New `internal/buildinfo/buildinfo.go` (must be its own package — `app.go` is `package main` and `internal/selfupdate` can't import it):

```go
const devVersion = "dev"
var Version = devVersion   // -X .../internal/buildinfo.Version=1.2.3
var Commit, Date string
func IsRelease() bool { return Version != devVersion && Version != "" }
func Display() string  // "dev" or Version
```

- `app.go`: delete `const AppVersion` (lines 121-124); `Ping()` returns `buildinfo.Display()`. JSON tag `appVersion` unchanged → `frontend/dist/js/views/settings.js:19` needs no edit.
- `main.go`: add a `--version` fast path before `wails.Run` (exit 0). Required by the pre-swap smoke test in M5.
- `wails.json`: add the missing `info` block — `productName: "Sourdot"` (becomes `%LOCALAPPDATA%\Programs\Sourdot`; **changing it later breaks install-kind detection**, so keep it the bare wordmark, *not* the tagline), `companyName`, `copyright`, `productVersion: "0.0.0"` (CI patches). Set `comments`/`fileDescription` to "Godot Version Manager" — that string is what Windows Start-menu search indexes beyond the shortcut name, and it is the Windows counterpart to the Linux `.desktop` `Keywords=` line. Name the NSIS Start-menu shortcut `Sourdot — Godot Version Manager` for the same reason.
- `Makefile`: `VERSION ?= dev`, `LDFLAGS := -X .../buildinfo.Version=$(VERSION)`, pass to `wails build`.
- ~~Add `LICENSE`~~ — done: GPL-3.0-or-later, which also unblocks the NSIS license page.

Dev builds report `dev` → `IsRelease()` false → the updater short-circuits before touching the network. That is the single gate stopping `wails dev` / `go run` from self-updating.

## Milestone 2 — Windows packaging (0.5d)

```
wails build -platform windows/amd64 -nsis -installscope user -ldflags "..."
```

- **amd64 only.** `wails.checkArchitecture` (`wails_tools.nsh:67-95`) hard-refuses to install on native ARM64 Windows with an amd64-only installer. Accepted for v1; ARM64 users can use the portable `.exe`.
- `-webview2 download` (default) embeds the ~2MB bootstrapper; with `REQUEST_EXECUTION_LEVEL=user` the macro correctly checks HKCU.
- Optional `project.nsi` edits: license page pointing at `LICENSE`, `MUI_FINISHPAGE_RUN`, and consider dropping the `$DESKTOP` shortcut.
- Harden `.github/workflows/build.yml`: pin `wails@v2.13.0` (today `@latest` can break CI with no repo change), add `cache: true` to `setup-go`, upload artifacts.

**Unsigned reality:** users get the "Windows protected your PC" dialog and must click *More info → Run anyway*; reputation is per-file-hash so it resets every release. Document it in the README and the release-notes header. Note that *self-updates are mostly immune* — the swapped `.exe` is written by a running process, so no MOTW is attached.

## Milestone 3 — Linux packaging (1–1.5d, highest packaging risk)

Create:
- `build/linux/sourdot.desktop` — relative `Exec=sourdot %U`, `Icon=sourdot`, `Categories=Development;IDE;`, and **`StartupWMClass=sourdot`** (without it GNOME/KDE show a generic task-switcher icon for Wails windows). Must also carry `GenericName=Godot Version Manager` and a `Keywords=Godot;GodotEngine;Godot4;Godot3;engine;version;manager;gamedev;launcher;` line — the name "Sourdot" is not self-describing, and GNOME/KDE app search match on Name/GenericName/Comment/Keywords, so without these a user searching "godot" never finds the app. Mirror whatever `Sourdot-Dev.desktop` uses; that file stays dev-only.
- `build/linux/icons/hicolor/{16,32,48,64,128,256,512}/apps/sourdot.png`
- `scripts/make-appimage.sh $VERSION` — assemble `AppDir` (binary at `usr/bin/`, desktop file + icon at root and under `usr/share/`), write a 6-line `AppRun`, then `appimagetool --runtime-file <pinned type2-runtime>`. Pin and sha256-check both downloads.

The modern type2-runtime statically links squashfuse, so **libfuse2 is not required on the host** — this removes the classic Fedora papercut.

Also add `App.InstallDesktopEntry()` (Linux only): writes `~/.local/share/applications/sourdot.desktop` with `Exec="$APPIMAGE" %U` + copies the icon. Because self-update keeps the new file at the *same path*, that `Exec=` stays valid across updates.

## Milestone 4 — Updater core (1.5d, fully offline-testable)

```
internal/selfupdate/
  updater.go feed.go manifest.go pubkey.go semver.go installkind.go
cmd/updatekeys/     # gen | manifest | sign  — stdlib crypto/ed25519 + crypto/sha256 only
```

`internal/selfupdate` must not import Wails (same discipline as `internal/core`); it takes a `core.EventSink`.

**Manifest** (`manifest.json`, signed as a whole by `manifest.json.sig` = base64 of the raw 64-byte Ed25519 sig):

```json
{ "schema": 1, "version": "1.2.3", "releaseURL": "...", "publishedAt": "...",
  "artifacts": [ { "kind": "appimage|exe|nsis", "os": "...", "arch": "amd64",
                   "name": "...", "url": "...", "size": 0, "sha256": "..." } ] }
```

One signed manifest beats per-artifact `.sig` files: one signing step in CI, the **version number itself is signed** (so stale-asset replay can't pin users to an old build), and per-artifact SHA-256 lets the download verify while streaming with no second fetch.

**Verification order is strict: verify the signature over the raw bytes *before* `json.Unmarshal`, before any download, before any comparison.** Signature failure = hard abort, never install.

`pubkey.go` holds a **slice** of base64 keys as a source constant (reviewable in diffs; can't be swapped by dropping a file next to the binary). Generate **two** keypairs at setup, embed both, keep the backup private seed offline — losing the CI key otherwise strands every install permanently.

**Version comparison:** pure semver in `semver.go` (~40 lines). `release.CompareLabels` is the wrong tool — it orders Godot's dev/alpha/beta/rc/stable vocabulary. Never downgrade even if `/latest/` regresses; skip if `version == settings.SkippedUpdateVersion`.

**Install-kind detection** (`installkind.go`), checked in this order:
1. `!buildinfo.IsRelease()` → `KindDev`, short-circuit.
2. Linux: `$APPIMAGE` set, points at an existing regular file, `$APPDIR` also set → `KindAppImage`, target `$APPIMAGE`. Probe writability by create+remove of a temp file in its directory (this simultaneously proves the same-filesystem rename will work).
3. Windows: `os.Executable()` → `EvalSymlinks` → case-insensitive dir compare against `%LOCALAPPDATA%\Programs\Sourdot` → `KindWindows`. `golang.org/x/sys` is already an indirect dep, so the optional HKCU uninstall-key corroboration adds no new module.
4. Else `KindPortable`.

Non-updatable kinds still show the banner, but with a single **"Open release page"** button → `App.OpenURL(manifest.releaseURL)`. `github.com` is already in `browserAllowedHosts`; take the URL from the *signed* manifest only.

**Settings additions** (`internal/store/settings.go`, all `omitempty`):
- `AutoCheckUpdates *bool` — **pointer is deliberate**: a plain `bool` would silently disable auto-check for every existing user, since their `settings.json` lacks the field and would read Go's `false` zero value.
- `UpdateLastCheckedAt time.Time`, `SkippedUpdateVersion string`.

Throttle `const updateCheckInterval = 24 * time.Hour` (the manifest is a ~1KB static CDN file, so cost isn't the driver — checking more often just re-nags). Independent of `releaseIndexTTL = 6h`. Delay the launch check ~5s after `domReady`. "Check now" passes `force: true`.

Small backwards-compatible addition to reuse `install.Download` with a timeout: extract `DownloadWithClient(ctx, *http.Client, url, dest, onProgress)` and have `Download` call it with `http.DefaultClient`. Zero call-site churn.

## Milestone 5 — Swap + relaunch (1d, highest correctness risk)

**Linux** (`swap_linux.go`): `os.CreateTemp` in `filepath.Dir(target)` (same FS → atomic rename) → download, hashing while streaming → compare SHA-256, mismatch aborts and does **not** retry → `chmod 0755` → **smoke-test `exec(tmp, "--version")` with a 5s timeout, requiring exit 0 and a version greater than current** → `os.Rename(target, target+".old")` (renames *aside*; never unlinks the live inode) → `os.Rename(tmp, target)`, restoring `.old` on failure.

The smoke test is the highest-value 10 lines in the plan: it converts the worst failure mode (bricked install because the host lacks webkit2gtk-4.1, or the AppImage won't mount) into a clean pre-swap abort.

**Windows** (`swap_windows.go`): same sequence, but rename the running `.exe` → `.old`, move the new one into place, set `FILE_ATTRIBUTE_HIDDEN` on `.old`, relaunch, delete `.old` at next startup. Chosen over "download the NSIS installer and run it `/S`" because that alternative forfeits all progress reporting and error visibility (we'd have to exit before it runs) and can half-install with the app gone. Its one genuine advantage — a correct `DisplayVersion` in Add/Remove Programs — is recovered in ~10 lines writing HKCU after a successful swap, no elevation needed.

**The relaunch trap — design around it explicitly.** `store.Open` uses `bolt.Options{Timeout: 1s}` (`internal/store/db.go:35`) and `app.startup` maps any open failure to `fatal()` + a native "another copy is already running" dialog. A naive `exec.Start(); Quit()` starts the child before the parent's `OnShutdown` runs `db.Close()`, so the child races a 1-second window. When it loses, the user clicks "Update & restart" and sees a *hard error dialog* — the update succeeded but looks catastrophic. Three layers:

1. **Spawn from `shutdown()`, after `db.Close()`** — reduces the overlap to zero rather than shrinking it. Add `App.relaunchAfterShutdown string`; `RelaunchForUpdate()` just calls `wailsruntime.Quit` (`beforeClose` already returns `false`).
2. Add `store.OpenWithTimeout(path, timeout)` and reduce `Open` to a 1s wrapper (**no existing call sites change**); `startup` uses 30s when the relaunch marker is present.
3. Marker via **environment** (`SOURDOT_RELAUNCHED_FROM=<oldVersion>`), not argv — argv is passed through by AppImage/shortcut invocations and can be typed by a confused user. `startup` reads it to pick the long timeout, delete the stale `.old`, and show an "Updated to vX.Y.Z" toast.

**AppImage relaunch also needs a sanitized child environment.** `AppRun` sets `APPDIR`, `APPIMAGE`, `OWD`, `ARGV0` and usually prepends the old mount's `usr/lib` to `LD_LIBRARY_PATH`. Inherited, the new instance may load shared libraries from a mountpoint that's about to be torn down. Strip `APPDIR`/`APPIMAGE`/`OWD`/`ARGV0`/`LD_LIBRARY_PATH`/`LD_PRELOAD`/`GTK_PATH`/`GDK_PIXBUF_MODULE_FILE`, and set `SysProcAttr{Setsid: true}` so the child survives the parent's process-group teardown.

**Not recoverable:** a new binary that crashes *after* a successful swap — the old process is gone, so auto-rollback is impossible. Mitigation is the pre-swap smoke test plus retaining `.old` for one extra launch, with the manual fix documented in the release notes. Deliberate non-goals: no watchdog, no A/B slots.

Events (all reusing `core.Event{Type, ID, Data}`, `ID` = target version): `update_available`, `update_download_progress`, `update_verified`, `update_ready`, `update_failed`.

## Milestone 6 — Frontend (0.5d)

Banner goes **inside `<main class="content">` but outside every `<section class="view">`** — `app.js`'s tab switcher only toggles `.view` elements, so it survives view changes. States on one element: `available` (Release notes / Skip / Update & restart) → `working` (progress bar) → `ready` (Restart now / Later) → `failed`. Never modal.

- New `frontend/dist/js/views/update.js`, same module shape as the existing three; `<script>` before `js/app.js`; `updateBanner.init()` alongside the other inits.
- **Add all five `update_*` events to the bind array at `frontend/dist/js/app.js:21`** — and while there, add the missing `project_auto_install_started` (emitted at `internal/core/projectmanager.go:198`, never bound → dead today). Consider a `go test` that greps `Event{Type: "..."}` literals across `internal/` and asserts each appears in `app.js`, so this bug can't recur.
- Settings: one new `.settings-panel` with an auto-check toggle, last-checked label + "Check now", and an honest install-kind line ("Installed as an AppImage — updates install automatically" / "Portable build — updates must be downloaded manually" / "Development build — updates disabled"). `#app-version` needs no change.
- `api.js`: `checkForUpdate(force)`, `applyUpdate()`, `relaunchForUpdate()`, `skipUpdateVersion(v)`, `getUpdatePreferences()`, `setAutoCheckUpdates(e)`, `installDesktopEntry()`.

**No channel selector in v1** — `/releases/latest/download/` already excludes prereleases, so "stable only" is free and correct.

## Milestone 7 — Release pipeline (1d)

`.github/workflows/release.yml`, `on: push: tags: ["v*"]`, `permissions: contents: write`:

- **`version` job** — derive and *validate* once against `^v\d+\.\d+\.\d+(-rc\.\d+)?$` (hard `exit 1`), emitting `VERSION`, `NSIS_VERSION` (numeric core only), `PRERELEASE`.
- **`linux` job** on `ubuntu-22.04` — apt webkit2gtk-4.1 dev headers, pinned `wails@v2.13.0`, `jq` patch of `wails.json`, `wails build -tags webkit2_41 -ldflags ...`, `make-appimage.sh`, tarball, upload-artifact.
- **`windows` job** — `wails build -platform windows/amd64 -nsis -installscope user -ldflags ...`, rename outputs, upload-artifact.
- **`publish` job**, `needs: [linux, windows]`, `environment: release` (gates the secret) — `updatekeys manifest` → `updatekeys sign` (seed read from `$UPDATE_SIGNING_KEY`, **never argv**) → `gh release create --generate-notes --notes-file .github/release-notes-header.md`, uploading artifacts + `manifest.json` + `.sig`. Depending on both build jobs is what prevents a half-published release that existing installs would try to fetch.

Cutting a release is `make release VERSION=1.2.3`, which guards on: version regex, clean working tree, tag doesn't already exist, `go vet` + `go test` pass — then tags and `git push --follow-tags`. The guards matter more than the brevity.

**One-time setup:** create the public repo; `updatekeys gen` twice (embed both public keys, `gh secret set UPDATE_SIGNING_KEY` with the primary seed, both private seeds to a password manager); create the `release` environment with a protection rule; add LICENSE; fill `wails.json` info; enable 2FA/passkey.

**Honest tradeoff of CI-side signing** (accepted, given the set-and-forget requirement): it protects against a compromised CDN/mirror, TLS MITM, and anyone with only `Releases: write` swapping an asset. It does **not** protect against full GitHub-account compromise or anyone who can push a workflow file — a workflow can trivially exfiltrate the secret. Offline signing closes that but makes every release a manual ritual. Cheap zero-friction hardening: the `release` environment gate, minimal `permissions:`, third-party actions pinned to commit SHAs, and never enabling `pull_request_target` in this repo.

## Verification

**Offline/unit (CI-gated):** table-driven tests for `semver.Compare`, `installkind.DetectInstall` (env-injected), manifest signature verification against a fixture keypair — including tampered-manifest, wrong-key, `schema: 2`, and no-matching-artifact cases. Plus the event-binding grep test.

**Dress rehearsal (the real end-to-end test):** `make release VERSION=0.9.0-rc.1` → published as a **prerelease**, so `/releases/latest/download/` ignores it and no real user is exposed. Install both artifacts by hand, then cut `0.9.1-rc.1` and drive the full update path: banner appears → download progress → verify → swap → relaunch → new version in Settings → `.old` cleaned up on the following launch.

**Requires real hardware / empirical checks:**
1. **All of Windows.** STATUS.md states nobody has ever run this app on real Windows; everything in M2 and the Windows swap is designed from docs. Verify: per-user NSIS install with no UAC, WebView2 bootstrapper on a machine lacking the runtime, SmartScreen click-through, renaming the running `.exe`, relaunch not hitting the bolt lock, `.old` cleanup.
2. **Renaming a running AppImage** (~15 min): launch it, `mv X X.old` from another terminal, click around. The rename-*aside* ordering is strictly safer than rename-over, but this is unconfirmed by any authoritative source.
3. **glibc floor** — confirm the `ubuntu-22.04`-built AppImage runs on both Fedora 44 and Ubuntu 22.04.
4. **FUSE on Fedora 44** — confirm the pinned type2-runtime really avoids the libfuse2 requirement.

## Effort

≈6–7 focused days: M1 0.5 · M2 0.5 · M3 1–1.5 · M4 1.5 · M5 1 · M6 0.5 · M7 1 · verification 0.5 + hardware.
