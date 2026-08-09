//go:build integration

package core_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/project"
	"github.com/gladelynch/sourdot/internal/store"
)

// TestOpenProject_EndToEnd exercises the full acceptance flow: add a real
// project folder, confirm the metadata parsed out of it, pin it (via
// .sourdot-version) to a version that isn't installed yet, confirm Open
// refuses rather than substituting whatever else is around, then install
// the version it asked for and open it for real. Network-dependent and
// starts a real (briefly-lived) Godot process, so it's gated the same way
// as the version-manager tests:
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
	pm := core.NewProjectManager(db, nil, vm, dir)
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

	pinPath := filepath.Join(projectDir, ".sourdot-version")
	if err := os.WriteFile(pinPath, []byte(targetVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// versionsDir is empty, so the pinned version isn't installed yet.
	// Resolution has to say exactly that, and Open has to refuse -- the
	// whole point of dropping the old auto-detect is that nothing gets
	// launched in a version the project didn't ask for, and nothing gets
	// downloaded without the user saying so.
	res := pm.ResolveVersion(proj)
	if res.Status != project.StatusMissing || res.Source != ".sourdot-version" {
		t.Fatalf("ResolveVersion = %+v; want status %q from .sourdot-version", res, project.StatusMissing)
	}
	if res.Version != targetVersion {
		t.Fatalf("resolved version = %q, want %s", res.Version, targetVersion)
	}
	if _, _, err := pm.OpenProject(ctx, proj.ID); err == nil {
		t.Fatal("OpenProject succeeded with no version installed; it must refuse instead")
	}

	// The explicit install step the UI offers once the user confirms.
	installed, err := pm.InstallForProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("InstallForProject: %v", err)
	}
	if installed.Version != targetVersion {
		t.Fatalf("installed version = %s, want %s", installed.Version, targetVersion)
	}

	iv, cmd, err := pm.OpenProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	if iv.Version != targetVersion {
		t.Fatalf("launched version = %s, want %s", iv.Version, targetVersion)
	}
	t.Logf("installed and launched %s (pid %d)", iv.ID, cmd.Process.Pid)

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
