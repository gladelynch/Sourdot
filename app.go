package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

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

	db             *store.DB
	dataDir        string
	versionManager *core.VersionManager
	projectManager *core.ProjectManager
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
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
		log.Printf("failed to stamp schema version: %v", err)
	}
	a.db = db

	settings, err := store.LoadSettings(dir)
	if err != nil {
		log.Printf("failed to load settings, using defaults: %v", err)
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
	log.Printf("%s: %v", message, err)
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
			log.Printf("failed to close store: %v", err)
		}
	}
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
	return a.versionManager.ListAvailableGrouped(a.ctx)
}

// RefreshAvailableVersions is ListAvailableVersions with a forced refetch,
// behind the Versions page's Refresh button.
func (a *App) RefreshAvailableVersions() ([]release.SeriesGroup, error) {
	return a.versionManager.RefreshAvailableGrouped(a.ctx)
}

// ListInstalledVersions returns every version currently installed on disk.
func (a *App) ListInstalledVersions() ([]install.InstalledVersion, error) {
	return a.versionManager.ListInstalled()
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
	rel, err := a.versionManager.FindRelease(a.ctx, tagName)
	if err != nil {
		return install.InstalledVersion{}, err
	}
	return a.versionManager.InstallVersion(a.ctx, rel, isMono)
}

// RemoveVersion deletes an installed version's files and its store record.
func (a *App) RemoveVersion(id string) error {
	return a.versionManager.RemoveVersion(id)
}

// GetDefaultVersion returns the InstalledVersion.ID used to resolve
// unpinned projects, or "" if none is set yet.
func (a *App) GetDefaultVersion() (string, error) {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		return "", err
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
	return store.SaveSettings(a.dataDir, settings)
}

// PickAndAddProject opens a native folder picker and, if the user selects
// one, adds it as a tracked project in one step. Returns nil (not an
// error) if the user cancels the picker.
func (a *App) PickAndAddProject() (*project.Project, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a Godot project folder",
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil // user cancelled
	}
	proj, err := a.projectManager.AddProject(dir)
	if err != nil {
		return nil, err
	}
	return &proj, nil
}

// ListProjects returns every tracked project.
func (a *App) ListProjects() ([]project.Project, error) {
	return a.projectManager.ListProjects()
}

// RemoveProject stops tracking a project (never touches its files on disk).
func (a *App) RemoveProject(id string) error {
	return a.projectManager.RemoveProject(id)
}

// SetFavorite toggles a project's favorite flag.
func (a *App) SetFavorite(id string, favorite bool) (project.Project, error) {
	return a.projectManager.SetFavorite(id, favorite)
}

// SetTags replaces a project's tag list.
func (a *App) SetTags(id string, tags []string) (project.Project, error) {
	return a.projectManager.SetTags(id, tags)
}

// SetPinnedVersion sets (or, with versionID "", clears) a project's
// explicit UI pin.
func (a *App) SetPinnedVersion(id, versionID string) (project.Project, error) {
	return a.projectManager.SetPinnedVersion(id, versionID)
}

// OpenProject resolves and (auto-installing if needed) launches the
// correct Godot editor for a tracked project. The launched editor process
// is intentionally not tracked here -- it's meant to keep running
// independently of Sourdot.
func (a *App) OpenProject(id string) (install.InstalledVersion, error) {
	iv, _, err := a.projectManager.OpenProject(a.ctx, id)
	return iv, err
}

// HasGitHubToken reports whether a GitHub PAT is currently saved, without
// exposing the token's value to the frontend.
func (a *App) HasGitHubToken() (bool, error) {
	settings, err := store.LoadSettings(a.dataDir)
	if err != nil {
		return false, err
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
		return err
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
		return fmt.Errorf("not a valid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing to open non-https URL %q", rawURL)
	}
	if !browserAllowedHosts[u.Hostname()] {
		return fmt.Errorf("refusing to open URL outside Godot's documented domains: %q", u.Hostname())
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
	return cmd.Start()
}
