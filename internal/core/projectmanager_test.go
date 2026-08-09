package core_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/install"
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
	pm1 := core.NewProjectManager(db1, nil, nil, "")

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

	pm2 := core.NewProjectManager(db2, nil, nil, "")
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

// seedVersion persists a fake installed record. Nothing here touches the
// filesystem: every behaviour under test (ordering, pin resolution) reads
// the store, not the versions directory.
func seedVersion(t *testing.T, db *store.DB, tag string, major, minor, patch int, label string) install.InstalledVersion {
	t.Helper()
	iv := install.InstalledVersion{
		ID:      tag + "-linux-x86_64",
		TagName: tag,
		Version: fmt.Sprintf("%d.%d.%d", major, minor, patch),
		Major:   major,
		Minor:   minor,
		Patch:   patch,
		Label:   label,
		OS:      "linux",
		Arch:    "x86_64",
	}
	if err := db.PutInstalledVersion(iv); err != nil {
		t.Fatalf("PutInstalledVersion(%s): %v", tag, err)
	}
	return iv
}

// TestListInstalledOrdersByRelease covers the ordering the installed shelf
// shows: the store hands records back in key order (effectively install
// order), which is meaningless to a user. Versions are seeded here in an
// order that is neither correct nor reverse-correct, so a pass can't come
// from the store happening to return them sorted.
//
// The 4.8 pre-releases are the case that matters most: they all share
// version 4.8.0, so only the label separates them.
func TestListInstalledOrdersByRelease(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-dev2", 4, 8, 0, "dev2")
	seedVersion(t, db, "4.7.1-stable", 4, 7, 1, "stable")
	seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")
	seedVersion(t, db, "3.6-stable", 3, 6, 0, "stable")
	seedVersion(t, db, "4.8-beta1", 4, 8, 0, "beta1")
	seedVersion(t, db, "4.7-stable", 4, 7, 0, "stable")

	vm := core.NewVersionManager(db, nil, t.TempDir(), "")
	installed, err := vm.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}

	var got []string
	for _, iv := range installed {
		got = append(got, iv.TagName)
	}
	want := []string{"4.8-beta1", "4.8-dev3", "4.8-dev2", "4.7.1-stable", "4.7-stable", "3.6-stable"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListInstalled order:\n got: %v\nwant: %v", got, want)
	}
}

// TestResolveVersionSpecPrefersDefaultVersion pins down the two halves of
// the "opened in the wrong build" bug: the default version set in Settings
// has to be consulted at all, and the spec it produces has to name the
// exact build, since 4.8-dev2 and 4.8-dev3 are indistinguishable by
// version number alone.
func TestResolveVersionSpecPrefersDefaultVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-dev2", 4, 8, 0, "dev2")
	dev3 := seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")
	if err := store.SaveSettings(dir, store.Settings{DefaultVersionID: dev3.ID}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "defaulted"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	spec, source, ok := pm.ResolveVersionSpec(proj)
	if !ok {
		t.Fatalf("ResolveVersionSpec returned ok=false, want the default version")
	}
	if spec.TagName != "4.8-dev3" {
		t.Errorf("resolved tag = %q (source %q), want 4.8-dev3", spec.TagName, source)
	}
}

// TestResolveVersionSpecFallsBackToNewestBuild covers the same project
// with no default set: resolution drops to the detected-major tier, which
// must pick the newest build in the series rather than whichever record
// the store returned first.
func TestResolveVersionSpecFallsBackToNewestBuild(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-dev2", 4, 8, 0, "dev2")
	seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")

	pm := core.NewProjectManager(db, nil, nil, dir) // dir has no settings.json: no default set
	proj, err := pm.AddProject(newProjectDir(t, dir, "undefaulted"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	spec, source, ok := pm.ResolveVersionSpec(proj)
	if !ok {
		t.Fatalf("ResolveVersionSpec returned ok=false, want the newest installed 4.x")
	}
	if spec.TagName != "4.8-dev3" {
		t.Errorf("resolved tag = %q (source %q), want 4.8-dev3", spec.TagName, source)
	}
}

// newProjectDir writes a minimal but real Godot 4 project.godot into a new
// subfolder of parent and returns its path.
func newProjectDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const projectGodot = "config_version=5\n\n[application]\n\nconfig/name=\"Pin Test\"\n"
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte(projectGodot), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
