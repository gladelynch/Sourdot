package main

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/gladelynch/sourdot/internal/applog"
	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/godot/release"
	"github.com/gladelynch/sourdot/internal/platform"
	"github.com/gladelynch/sourdot/internal/project"
	"github.com/gladelynch/sourdot/internal/store"
)

// App is the thin Wails binding layer. It owns process lifecycle (opening
// and closing the store) and adapts core.Event to Wails runtime events; all
// real logic lives in internal/core so a future CLI/TUI can reuse it
// without this file.
type App struct {
	ctx context.Context

	log            *applog.Logger
	db             *store.DB
	dataDir        string
	versionManager *core.VersionManager
	projectManager *core.ProjectManager
}

// NewApp creates a new App application struct.
func NewApp(log *applog.Logger) *App {
	return &App{log: log}
}

// logErr records an error on its way back to the frontend and returns it
// unchanged, so bindings stay one-liners.
//
// Every binding below hands its error straight to JS, where it becomes an
// alert() string and nothing else -- so without this the interesting half
// of every failure (which call, what the wrapped cause was) only ever
// existed in a dialog the user had already dismissed.
func (a *App) logErr(op string, err error) error {
	if err != nil {
		a.log.Errorf("%s: %v", op, err)
	}
	return err
}

// Emit implements core.EventSink by forwarding to the Wails runtime.
func (a *App) Emit(evt core.Event) {
	wailsruntime.EventsEmit(a.ctx, evt.Type, evt)
}

// startup is called at application startup.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dir, err := platform.ConfigDir()
	if err != nil {
		a.fatal("Sourdot couldn't resolve its configuration directory.", err)
		return
	}
	a.dataDir = dir

	db, err := store.Open(filepath.Join(dir, "sourdot.db"))
	if err != nil {
		a.fatal("Sourdot couldn't open its database.\n\n"+
			"This usually means another copy of Sourdot is already running.", err)
		return
	}
	if err := db.EnsureSchemaVersion(); err != nil {
		a.log.Errorf("failed to stamp schema version: %v", err)
	}
	a.db = db

	settings, err := store.LoadSettings(dir)
	if err != nil {
		a.log.Errorf("failed to load settings, using defaults: %v", err)
		settings = store.DefaultSettings()
	}

	versionsDir, err := platform.VersionsDir()
	if err != nil {
		a.fatal("Sourdot couldn't create its versions directory.", err)
		return
	}

	a.versionManager = core.NewVersionManager(db, a, versionsDir, settings.GitHubToken)
	a.projectManager = core.NewProjectManager(db, a, a.versionManager, dir)
}

// fatal reports an unrecoverable startup failure and exits.
//
// Every binding below assumes startup finished wiring the managers up.
// Before this existed, a startup failure just logged and returned, leaving
// them nil so the first call from the frontend nil-panicked -- the user saw
// an empty, silently broken window with no explanation. The most likely
// trigger is mundane: a second copy of Sourdot finding the BoltDB file
// already locked.
func (a *App) fatal(message string, err error) {
	a.log.Errorf("%s: %v", message, err)
	_, _ = wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.ErrorDialog,
		Title:   "Sourdot can't start",
		Message: fmt.Sprintf("%s\n\n%v", message, err),
	})
	wailsruntime.Quit(a.ctx)
}

// domReady is called after front-end resources have been loaded.
func (a *App) domReady(ctx context.Context) {}

// beforeClose is called when the application is about to quit, either by
// clicking the window close button or calling runtime.Quit. Returning true
// prevents the close; Sourdot has no unsaved-state prompt to show, so it
// always allows it.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// shutdown is called at application termination.
func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.log.Errorf("failed to close store: %v", err)
		}
	}
	a.log.Printf("Sourdot shutting down")
}

// AppVersion is Sourdot's own version string. v1 has no release/tagging
// process yet (packaging/signing/auto-update are explicitly deferred past
// v1 per the plan), so this stays a hand-bumped placeholder for now.
const AppVersion = "0.1.0-dev"

// PingResult is returned by Ping as a smoke test that the Go<->JS binding
// and store are both alive; the Settings view renders it.
type PingResult struct {
	SchemaVersion int    `json:"schemaVersion"`
	DataDir       string `json:"dataDir"`
	AppVersion    string `json:"appVersion"`
}

// Ping reports basic backend health for the Settings view.
func (a *App) Ping() PingResult {
	return PingResult{
		SchemaVersion: store.SchemaVersion,
		DataDir:       a.dataDir,
		AppVersion:    AppVersion,
	}
}

// ListAvailableVersions returns the full Godot catalog -- stable, rc, beta,
// alpha and dev -- grouped into version series for the Versions page,
// served from the local cache when it's fresh.
func (a *App) ListAvailableVersions() ([]release.SeriesGroup, error) {
	groups, err := a.versionManager.ListAvailableGrouped(a.ctx)
	return groups, a.logErr("ListAvailableVersions", err)
}

// RefreshAvailableVersions is ListAvailableVersions with a forced refetch,
// behind the Versions page's Refresh button.
func (a *App) RefreshAvailableVersions() ([]release.SeriesGroup, error) {
	groups, err := a.versionManager.RefreshAvailableGrouped(a.ctx)
	return groups, a.logErr("RefreshAvailableVersions", err)
}

// ListInstalledVersions returns every version currently installed on disk.
func (a *App) ListInstalledVersions() ([]install.InstalledVersion, error) {
	installed, err := a.versionManager.ListInstalled()
	return installed, a.logErr("ListInstalledVersions", err)
}

// InstallVersion downloads and installs the release tagged tagName, in the
// requested standard/mono variant, picking the asset matching this host's
// platform. Emits download_progress/checksum_verified/install_complete
// events while it runs, which the Versions view uses to drive its progress
// bar.
//
// Takes a tag rather than a Release value so the frontend never has to
// round-trip a struct it can't fully see: the catalog it holds is trimmed
// for size, and the backend re-resolves the complete record here.
func (a *App) InstallVersion(tagName string, isMono bool) (install.InstalledVersion, error) {
	a.log.Printf("InstallVersion: tag=%s mono=%t", tagName, isMono)
	rel, err := a.versionManager.FindRelease(a.ctx, tagName)
	if err != nil {
		return install.InstalledVersion{}, a.logErr("InstallVersion", err)
	}
	iv, err := a.versionManager.InstallVersion(a.ctx, rel, isMono)
	return iv, a.logErr("InstallVersion", err)
}

// RemoveVersion deletes an installed version's files and its store record.
func (a *App) RemoveVersion(id string) error {
	return a.logErr("RemoveVersion", a.versionManager.RemoveVersion(id))
}

// GetDefaultVersion returns the InstalledVersion.ID used to resolve
// unpinned projects, or "" if none is set yet.
func (a *App) GetDefaultVersion() (string, error) {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		return "", a.logErr("GetDefaultVersion", err)
	}
	return settings.DefaultVersionID, nil
}

// SetDefaultVersion sets the InstalledVersion.ID used to resolve unpinned
// projects.
func (a *App) SetDefaultVersion(id string) error {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		settings = store.DefaultSettings()
	}
	settings.DefaultVersionID = id
	return a.logErr("SetDefaultVersion", store.SaveSettings(a.dataDir, settings))
}

// PickAndAddProject opens a native folder picker and, if the user selects
// one, adds it as a tracked project in one step. Returns nil (not an
// error) if the user cancels the picker.
func (a *App) PickAndAddProject() (*project.Project, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a Godot project folder",
	})
	if err != nil {
		return nil, a.logErr("PickAndAddProject", err)
	}
	if dir == "" {
		return nil, nil // user cancelled
	}
	proj, err := a.projectManager.AddProject(dir)
	if err != nil {
		return nil, a.logErr("PickAndAddProject("+dir+")", err)
	}
	return &proj, nil
}

// ListProjects returns every tracked project.
func (a *App) ListProjects() ([]project.Project, error) {
	projects, err := a.projectManager.ListProjects()
	return projects, a.logErr("ListProjects", err)
}

// RemoveProject stops tracking a project (never touches its files on disk).
func (a *App) RemoveProject(id string) error {
	return a.logErr("RemoveProject", a.projectManager.RemoveProject(id))
}

// SetFavorite toggles a project's favorite flag.
func (a *App) SetFavorite(id string, favorite bool) (project.Project, error) {
	proj, err := a.projectManager.SetFavorite(id, favorite)
	return proj, a.logErr("SetFavorite", err)
}

// SetTags replaces a project's tag list.
func (a *App) SetTags(id string, tags []string) (project.Project, error) {
	proj, err := a.projectManager.SetTags(id, tags)
	return proj, a.logErr("SetTags", err)
}

// SetVersionMode switches how a project picks its editor. mode is one of
// "project", "default", or "pinned"; versionID is required for "pinned"
// and ignored otherwise.
func (a *App) SetVersionMode(id, mode, versionID string) (project.Project, error) {
	proj, err := a.projectManager.SetVersionMode(id, mode, versionID)
	return proj, a.logErr(fmt.Sprintf("SetVersionMode(mode=%s, version=%s)", mode, versionID), err)
}

// InstallForProject fetches the Godot build a project resolves to, without
// launching it. This is what the Projects page's "Install it" button calls
// when a row reports the version it needs isn't on disk; ProjectManager has
// had the method since the remediation workflow landed, but it was never
// bound, so the button failed with "not a function" before reaching Go.
func (a *App) InstallForProject(id string) (install.InstalledVersion, error) {
	iv, err := a.projectManager.InstallForProject(a.ctx, id)
	if err == nil {
		a.log.Printf("InstallForProject: installed %s for project %s", iv.ID, id)
	}
	return iv, a.logErr("InstallForProject", err)
}

// OpenProject resolves and (auto-installing if needed) launches the
// correct Godot editor for a tracked project. The launched editor process
// is intentionally not tracked here -- it's meant to keep running
// independently of Sourdot.
func (a *App) OpenProject(id string) (install.InstalledVersion, error) {
	iv, _, err := a.projectManager.OpenProject(a.ctx, id)
	if err == nil {
		a.log.Printf("OpenProject: launched %s for project %s", iv.ID, id)
	}
	return iv, a.logErr("OpenProject", err)
}

// HasGitHubToken reports whether a GitHub PAT is currently saved, without
// exposing the token's value to the frontend.
func (a *App) HasGitHubToken() (bool, error) {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		return false, a.logErr("HasGitHubToken", err)
	}
	return settings.GitHubToken != "", nil
}

// SetGitHubToken saves a GitHub PAT (used to raise the 60/hr unauthenticated
// API rate limit) and applies it immediately, without requiring an app
// restart. An empty token clears it.
func (a *App) SetGitHubToken(token string) error {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		settings = store.DefaultSettings()
	}
	settings.GitHubToken = token
	if err := store.SaveSettings(a.dataDir, settings); err != nil {
		return a.logErr("SetGitHubToken", err)
	}
	a.versionManager.UpdateGitHubToken(token)
	return nil
}

// browserAllowedHosts bounds what OpenURL will hand to the user's browser.
//
// The URLs the Versions page offers are parsed out of GitHub release
// bodies, which is remote content Sourdot doesn't author. Handing an
// arbitrary string straight to the OS URL handler would turn that into a
// way to launch non-http schemes or arbitrary sites from a link the user
// reasonably expects to be Godot documentation, so the target host is
// checked against the three domains Godot actually publishes on.
var browserAllowedHosts = map[string]bool{
	"godotengine.org":       true,
	"www.godotengine.org":   true,
	"github.com":            true,
	"godotengine.github.io": true,
}

// OpenURL opens a Godot release-notes or changelog URL in the user's
// default browser. Deliberately never renders remote pages in-app.
func (a *App) OpenURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return a.logErr("OpenURL", fmt.Errorf("not a valid URL: %w", err))
	}
	if u.Scheme != "https" {
		return a.logErr("OpenURL", fmt.Errorf("refusing to open non-https URL %q", rawURL))
	}
	if !browserAllowedHosts[u.Hostname()] {
		return a.logErr("OpenURL", fmt.Errorf("refusing to open URL outside Godot's documented domains: %q", u.Hostname()))
	}
	wailsruntime.BrowserOpenURL(a.ctx, u.String())
	return nil
}

// OpenDataDir opens Sourdot's data directory in the OS file manager.
func (a *App) OpenDataDir() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", a.dataDir)
	case "darwin":
		cmd = exec.Command("open", a.dataDir)
	default:
		cmd = exec.Command("xdg-open", a.dataDir)
	}
	return a.logErr("OpenDataDir", cmd.Start())
}

// LogFrontend records a message from the webview in Sourdot's log file.
//
// The frontend's own console goes to the WebKit inspector, which a built
// app doesn't have -- so before this, every caught JS error and every
// unhandled rejection was discarded the moment its alert() was dismissed.
// Routing them here puts frontend and backend failures in one file, in
// order, which is the only way to see that a JS TypeError and the Go call
// it came from are the same incident.
func (a *App) LogFrontend(level, message string) {
	switch level {
	case "error":
		a.log.Errorf("[webview] %s", message)
	case "warn":
		a.log.Warning(fmt.Sprintf("[webview] %s", message))
	default:
		a.log.Printf("[webview] %s", message)
	}
}

// GetLogPath returns the absolute path of the log file, or "" when Sourdot
// is running without one. The Settings view shows it so a user filing a bug
// knows what to attach.
func (a *App) GetLogPath() string {
	return a.log.Path()
}
