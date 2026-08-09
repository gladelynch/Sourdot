package core

import "github.com/gladelynch/godotvm/internal/store"

// VersionManager orchestrates Godot version discovery, install, and
// removal. It is UI-agnostic by design: app.go's Wails App struct is a thin
// adapter over this, and a future CLI/TUI (backlog) would bind directly to
// it instead of duplicating the logic.
//
// M1 fills in the discovery/install pipeline (a release.Client and the
// versions directory); this is just the wiring skeleton for now.
type VersionManager struct {
	db     *store.DB
	events EventSink
}

// NewVersionManager constructs a VersionManager.
func NewVersionManager(db *store.DB, events EventSink) *VersionManager {
	return &VersionManager{db: db, events: events}
}
