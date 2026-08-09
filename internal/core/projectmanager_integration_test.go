//go:build integration

package core_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gladelynch/godotvm/internal/core"
	"github.com/gladelynch/godotvm/internal/store"
)

// TestOpenProject_EndToEnd exercises the full M3 acceptance flow: add a
// real project folder, confirm its auto-detected metadata, pin it (via
// .godotvm-version) to a version that isn't installed yet, open it, and
// confirm that auto-installs the missing version and then launches the
// editor. Network-dependent and starts a real (briefly-lived) Godot
// process, so it's gated the same way as the version-manager tests:
//
//	go test -tags integration ./internal/core/... -run OpenProject -v
func TestOpenProject_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	versionsDir := filepath.Join(dir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	vm := core.NewVersionManager(db, nil, versionsDir, "")
	pm := core.NewProjectManager(db, nil, vm)
	ctx := context.Background()

	// A minimal but real project.godot -- config_version=5 is the actual
	// signal Godot 4.x editors write.
	projectDir := filepath.Join(dir, "myproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const projectGodot = `; Engine configuration file.
config_version=5

[application]

config/name="My Test Game"
config/features=PackedStringArray("4.2", "Forward Plus")
`
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte(projectGodot), 0o644); err != nil {
		t.Fatal(err)
	}

	proj, err := pm.AddProject(projectDir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if proj.Name != "My Test Game" {
		t.Errorf("Name = %q, want %q", proj.Name, "My Test Game")
	}
	if proj.DetectedVersion != "4" {
		t.Errorf("DetectedVersion = %q, want %q", proj.DetectedVersion, "4")
	}
	if proj.UsesCSharp {
		t.Errorf("UsesCSharp = true, want false")
	}

	releases, err := vm.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("expected at least one stable release")
	}
	target := releases[0]
	targetVersion := fmt.Sprintf("%d.%d.%d", target.Major, target.Minor, target.Patch)
	t.Logf("pinning project to %s", targetVersion)

	pinPath := filepath.Join(projectDir, ".godotvm-version")
	if err := os.WriteFile(pinPath, []byte(targetVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, source, ok := pm.ResolveVersionSpec(proj)
	if !ok || spec.Version != targetVersion || source != ".godotvm-version" {
		t.Fatalf("ResolveVersionSpec = %+v, %q, %v; want version %s from .godotvm-version", spec, source, ok, targetVersion)
	}

	// versionsDir is empty, so this must exercise the auto-install path.
	iv, cmd, err := pm.OpenProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	if iv.Version != targetVersion {
		t.Fatalf("installed version = %s, want %s", iv.Version, targetVersion)
	}
	t.Logf("auto-installed and launched %s (pid %d)", iv.ID, cmd.Process.Pid)

	// Don't let a real editor window linger in the user's session --
	// confirm it started, then kill it immediately.
	if err := cmd.Process.Kill(); err != nil {
		t.Logf("killing launched editor: %v (may have already exited)", err)
	}
	_ = cmd.Wait()

	updated, found, err := db.GetProject(proj.ID)
	if err != nil || !found {
		t.Fatalf("GetProject after open: found=%v err=%v", found, err)
	}
	if updated.LastOpenedAt.IsZero() {
		t.Error("LastOpenedAt was not updated after OpenProject")
	}

	if err := vm.RemoveVersion(iv.ID); err != nil {
		t.Fatalf("RemoveVersion cleanup: %v", err)
	}
}
