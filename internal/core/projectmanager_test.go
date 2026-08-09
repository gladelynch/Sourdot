package core_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/store"
)

// TestFavoritesAndTagsSurviveRestart targets the exact bug class flagged
// in the plan's competitor research (a popular Godot GUI tool resets
// favorites on restart): it closes and reopens the real BoltDB file --
// not just checking in-memory state -- to faithfully simulate an app
// restart, and confirms Favorite/Tags persisted. Offline and fast, so it
// runs as part of the normal `go test ./...` suite rather than needing
// the integration tag.
func TestFavoritesAndTagsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	projectDir := filepath.Join(dir, "myproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const projectGodot = "config_version=5\n\n[application]\n\nconfig/name=\"Restart Test\"\n"
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte(projectGodot), 0o644); err != nil {
		t.Fatal(err)
	}

	db1, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	// versions is nil here deliberately: AddProject/SetFavorite/SetTags
	// never touch it (only OpenProject's auto-install path would), so a
	// nil VersionManager is a safe stand-in and keeps this test network-free.
	pm1 := core.NewProjectManager(db1, nil, nil)

	proj, err := pm1.AddProject(projectDir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	if _, err := pm1.SetFavorite(proj.ID, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	wantTags := []string{"jam", "2026"}
	if _, err := pm1.SetTags(proj.ID, wantTags); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("closing db (simulating app shutdown): %v", err)
	}

	// Reopen the same on-disk file -- simulates relaunching the app.
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open (simulating app restart): %v", err)
	}
	defer db2.Close()

	pm2 := core.NewProjectManager(db2, nil, nil)
	projects, err := pm2.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects after restart: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects after restart, want 1", len(projects))
	}

	got := projects[0]
	if !got.Favorite {
		t.Error("Favorite did not survive restart")
	}
	if !reflect.DeepEqual(got.Tags, wantTags) {
		t.Errorf("Tags after restart = %v, want %v", got.Tags, wantTags)
	}
}
