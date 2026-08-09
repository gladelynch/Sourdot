package core

import "github.com/gladelynch/godotvm/internal/store"

// ProjectManager orchestrates project scanning, pin resolution, and the
// resolve-then-launch flow (find the pinned InstalledVersion, auto-install
// it via VersionManager if missing, then exec the Godot binary). UI-agnostic
// for the same reason as VersionManager.
//
// M3 fills in scanning/parsing/pin-resolution; this is just the wiring
// skeleton for now.
type ProjectManager struct {
	db     *store.DB
	events EventSink
}

// NewProjectManager constructs a ProjectManager.
func NewProjectManager(db *store.DB, events EventSink) *ProjectManager {
	return &ProjectManager{db: db, events: events}
}
