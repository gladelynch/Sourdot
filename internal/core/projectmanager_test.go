package core_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/project"
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

// TestNewProjectSticksToItsDeclaredVersion is the central guarantee of
// version resolution, and the reason the old "newest installed build of
// the same major" fallback was removed: opening a project in a newer
// editor makes Godot rewrite its files one-way, so a freshly added project
// must resolve to the version it was last saved in even when much newer
// builds are sitting installed and sorted ahead of it.
func TestNewProjectSticksToItsDeclaredVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-stable", 4, 8, 0, "stable")
	seedVersion(t, db, "4.2-stable", 4, 2, 0, "stable")
	seedVersion(t, db, "4.7.1-stable", 4, 7, 1, "stable")

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "legacy", "4.2"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if got := project.ResolveMode(proj); got != project.ModeProject {
		t.Errorf("new project mode = %q, want %q", got, project.ModeProject)
	}

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusReady {
		t.Fatalf("status = %q (%s), want ready", res.Status, res.Warning)
	}
	if res.Label != "4.2-stable" {
		t.Errorf("resolved to %q from %q, want 4.2-stable", res.Label, res.Source)
	}
	if res.WillUpgrade {
		t.Error("WillUpgrade = true for a project opening in its own version")
	}
}

// TestMissingDeclaredVersionIsReportedNotSubstituted covers the same
// project with its own version absent: resolution must say what's missing
// rather than quietly reaching for the newest 4.x on the shelf.
func TestMissingDeclaredVersionIsReportedNotSubstituted(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-dev2", 4, 8, 0, "dev2")
	seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "unavailable", "4.2"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusMissing {
		t.Fatalf("status = %q (resolved to %q), want missing", res.Status, res.Label)
	}
	if res.Version != "4.2" {
		t.Errorf("missing version = %q, want 4.2", res.Version)
	}
	if res.VersionID != "" {
		t.Errorf("VersionID = %q, want empty -- a missing version must not name a build to launch", res.VersionID)
	}

	// And the launch path has to refuse on its own, not just rely on the
	// UI having checked first.
	if _, _, err := pm.OpenProject(context.Background(), proj.ID); err == nil {
		t.Fatal("OpenProject succeeded for a project whose version isn't installed")
	}
}

// TestDeclaredVersionMatchesPatchRelease pins the one substitution that is
// safe: config/features records only "4.3", and patch releases within a
// series don't change the project format, so 4.3.1 satisfies a project
// declaring 4.3.
func TestDeclaredVersionMatchesPatchRelease(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.3-stable", 4, 3, 0, "stable")
	seedVersion(t, db, "4.3.1-stable", 4, 3, 1, "stable")
	seedVersion(t, db, "4.4-stable", 4, 4, 0, "stable")

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "patched", "4.3"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusReady || res.Label != "4.3.1-stable" {
		t.Errorf("resolved to %q (%s), want 4.3.1-stable", res.Label, res.Status)
	}
	if res.WillUpgrade {
		t.Error("WillUpgrade = true across a patch release within the declared series")
	}
}

// TestUndeclaredVersionRequiresAChoice covers Godot 3 projects, which
// write no config/features line at all. A major version is not a version,
// so resolution reports that nothing has been chosen rather than picking
// the newest 3.x -- 3.5 opened in 3.6 is still a rewrite.
func TestUndeclaredVersionRequiresAChoice(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "3.5.3-stable", 3, 5, 3, "stable")
	seedVersion(t, db, "3.6-stable", 3, 6, 0, "stable")

	projectDir := filepath.Join(dir, "godot3")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const godot3 = "config_version=4\n\n[application]\n\nconfig/name=\"Old Game\"\n"
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte(godot3), 0o644); err != nil {
		t.Fatal(err)
	}

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(projectDir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if proj.DetectedVersion != "3" {
		t.Fatalf("DetectedVersion = %q, want 3", proj.DetectedVersion)
	}

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusUnknown {
		t.Fatalf("status = %q (resolved to %q), want unknown", res.Status, res.Label)
	}
	if res.Warning == "" {
		t.Error("unresolved project has no warning explaining what to do")
	}
	if _, _, err := pm.OpenProject(context.Background(), proj.ID); err == nil {
		t.Fatal("OpenProject succeeded for a project with no chosen version")
	}
}

// TestDefaultVersionOnlyAppliesWhenChosen covers the second half of the
// "opened in the wrong build" bug. The default version must be reachable
// -- naming the exact build, since 4.8-dev2 and 4.8-dev3 are identical by
// version number -- but only for a project explicitly switched to it, and
// opening a 4.2 project there has to be flagged as the one-way upgrade it
// is.
func TestDefaultVersionOnlyAppliesWhenChosen(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-dev2", 4, 8, 0, "dev2")
	dev3 := seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")
	seedVersion(t, db, "4.2-stable", 4, 2, 0, "stable")
	if err := store.SaveSettings(dir, store.Settings{DefaultVersionID: dev3.ID}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "defaulted", "4.2"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// Untouched, the default must not apply: the project declares 4.2.
	if res := pm.ResolveVersion(proj); res.Label != "4.2-stable" {
		t.Errorf("unchanged project resolved to %q, want 4.2-stable -- the default must not apply on its own", res.Label)
	}

	proj, err = pm.SetVersionMode(proj.ID, project.ModeDefault, "")
	if err != nil {
		t.Fatalf("SetVersionMode: %v", err)
	}
	proj.DeclaredVersion = "4.2" // as ListProjects/OpenProject refresh it from disk

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusReady {
		t.Fatalf("status = %q, want ready", res.Status)
	}
	if res.Label != "4.8-dev3" {
		t.Errorf("resolved to %q, want the exact default build 4.8-dev3", res.Label)
	}
	if !res.WillUpgrade {
		t.Error("WillUpgrade = false opening a 4.2 project in 4.8 -- this is the irreversible case")
	}
}

// TestLegacyPinKeepsWorking covers records written before VersionMode
// existed: a project that carried an explicit pin has to keep honouring
// it, rather than silently reverting to its declared version on upgrade.
func TestLegacyPinKeepsWorking(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	dev3 := seedVersion(t, db, "4.8-dev3", 4, 8, 0, "dev3")
	seedVersion(t, db, "4.2-stable", 4, 2, 0, "stable")

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "legacypin", "4.2"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// Rewrite the record the way an older Sourdot would have stored it.
	proj.VersionMode = ""
	proj.PinnedVersionID = dev3.ID
	if err := db.PutProject(proj); err != nil {
		t.Fatalf("PutProject: %v", err)
	}

	if got := project.ResolveMode(proj); got != project.ModePinned {
		t.Fatalf("legacy pinned record resolved to mode %q, want %q", got, project.ModePinned)
	}
	if res := pm.ResolveVersion(proj); res.Label != "4.8-dev3" {
		t.Errorf("legacy pin resolved to %q, want 4.8-dev3", res.Label)
	}
}

// TestResolutionLabelsAreNeverTruncated guards the rule that a version is
// never shown with its build label stripped off. "4.8" names four
// different builds -- 4.8-dev3, 4.8-beta1, 4.8-rc1, 4.8-stable -- so a
// label reading exactly "4.8" is always a bug: either it should be the
// resolved build's full release tag, or, when no single build is settled
// on, the series written as "4.8.x" so it reads as the range it is.
func TestResolutionLabelsAreNeverTruncated(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "labels", "4.8"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// Nothing installed: the label has to name the series, not a version.
	if res := pm.ResolveVersion(proj); res.Label != "4.8.x" {
		t.Errorf("uninstalled label = %q, want 4.8.x", res.Label)
	}

	// With a build installed, the label has to be its full release tag --
	// the version number alone can't tell rc1 from stable.
	for _, tc := range []struct {
		tag   string
		label string
	}{
		{"4.8-beta1", "beta1"},
		{"4.8-rc1", "rc1"},
		{"4.8-stable", "stable"},
	} {
		iv := seedVersion(t, db, tc.tag, 4, 8, 0, tc.label)
		res := pm.ResolveVersion(proj)
		if res.Status != project.StatusReady {
			t.Fatalf("%s: status = %q, want ready", tc.tag, res.Status)
		}
		if res.Label != tc.tag {
			t.Errorf("with %s installed, label = %q, want the full tag %q", tc.tag, res.Label, tc.tag)
		}
		if res.VersionID != iv.ID {
			t.Errorf("with %s installed, resolved build = %q, want %q -- the most stable build in the series", tc.tag, res.VersionID, iv.ID)
		}
	}
}

// TestUnshippedSeriesResolvesToItsLatestBuild pins what a project means
// when it records "4.8" while 4.8 is still in pre-release: the most recent
// 4.8 build there is. A series that hasn't shipped has no stable build to
// hold out for, so treating a pre-release match as second-best would leave
// the project with nothing to open in at all.
func TestUnshippedSeriesResolvesToItsLatestBuild(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	seedVersion(t, db, "4.8-beta1", 4, 8, 0, "beta1")
	seedVersion(t, db, "4.8-rc1", 4, 8, 0, "rc1")
	seedVersion(t, db, "4.8-dev5", 4, 8, 0, "dev5")

	pm := core.NewProjectManager(db, nil, nil, dir)
	proj, err := pm.AddProject(newProjectDir(t, dir, "prerelease", "4.8"))
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusReady {
		t.Fatalf("status = %q (%s), want ready", res.Status, res.Warning)
	}
	if res.Label != "4.8-rc1" {
		t.Errorf("resolved to %q, want the latest 4.8 build, 4.8-rc1", res.Label)
	}
	if res.Warning != "" {
		t.Errorf("opening a 4.8 project in the latest 4.8 build warns: %q", res.Warning)
	}
	if res.WillUpgrade {
		t.Error("WillUpgrade = true within the project's own declared series")
	}

	// Once the series ships, "4.8" means the stable build.
	seedVersion(t, db, "4.8-stable", 4, 8, 0, "stable")
	if res := pm.ResolveVersion(proj); res.Label != "4.8-stable" {
		t.Errorf("after 4.8 shipped, resolved to %q, want 4.8-stable", res.Label)
	}

	// ...and stays there rather than jumping to the next release candidate.
	seedVersion(t, db, "4.8.1-rc1", 4, 8, 1, "rc1")
	if res := pm.ResolveVersion(proj); res.Label != "4.8-stable" {
		t.Errorf("resolved to %q once 4.8.1-rc1 existed, want to stay on 4.8-stable", res.Label)
	}

	// A shipped patch release is where it should move next.
	seedVersion(t, db, "4.8.1-stable", 4, 8, 1, "stable")
	if res := pm.ResolveVersion(proj); res.Label != "4.8.1-stable" {
		t.Errorf("resolved to %q, want the shipped patch release 4.8.1-stable", res.Label)
	}
}

// newProjectDir writes a minimal but real Godot 4 project.godot into a new
// subfolder of parent and returns its path. declared is the version its
// config/features records -- exactly what Godot itself writes there, and
// the signal resolution now runs on.
func newProjectDir(t *testing.T, parent, name, declared string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	projectGodot := fmt.Sprintf(
		"config_version=5\n\n[application]\n\nconfig/name=\"Pin Test\"\nconfig/features=PackedStringArray(%q, \"Forward Plus\")\n",
		declared,
	)
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte(projectGodot), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
