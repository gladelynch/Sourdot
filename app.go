package main

import (
	"context"
	"log"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/gladelynch/godotvm/internal/core"
	"github.com/gladelynch/godotvm/internal/platform"
	"github.com/gladelynch/godotvm/internal/store"
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
		log.Printf("failed to resolve config dir: %v", err)
		return
	}
	a.dataDir = dir

	db, err := store.Open(filepath.Join(dir, "godotvm.db"))
	if err != nil {
		log.Printf("failed to open store: %v", err)
		return
	}
	if err := db.EnsureSchemaVersion(); err != nil {
		log.Printf("failed to stamp schema version: %v", err)
	}
	a.db = db

	a.versionManager = core.NewVersionManager(db, a)
	a.projectManager = core.NewProjectManager(db, a)
}

// domReady is called after front-end resources have been loaded.
func (a *App) domReady(ctx context.Context) {}

// beforeClose is called when the application is about to quit, either by
// clicking the window close button or calling runtime.Quit. Returning true
// prevents the close; GodotVM has no unsaved-state prompt to show, so it
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

// PingResult is returned by Ping as a smoke test that the Go<->JS binding
// and store are both alive; the Settings view renders it in M0.
type PingResult struct {
	SchemaVersion int    `json:"schemaVersion"`
	DataDir       string `json:"dataDir"`
}

// Ping reports basic backend health for the Settings view.
func (a *App) Ping() PingResult {
	return PingResult{
		SchemaVersion: store.SchemaVersion,
		DataDir:       a.dataDir,
	}
}
