package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/project"
	"github.com/gladelynch/sourdot/internal/shim"
	"github.com/gladelynch/sourdot/internal/store"
)

// TestShimTableFollowsOpenRules checks the launcher gives each project the
// answer the Open button would, and is stricter only where a headless run
// can't ask first: a project that would be upgraded is refused.
func TestShimTableFollowsOpenRules(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	withBinary := func(iv install.InstalledVersion) install.InstalledVersion {
		iv.BinaryPath = "/engines/" + iv.ID + "/godot"
		if err := db.PutInstalledVersion(iv); err != nil {
			t.Fatal(err)
		}
		return iv
	}
	v42 := withBinary(seedVersion(t, db, "4.2-stable", 4, 2, 0, "stable"))
	v48 := withBinary(seedVersion(t, db, "4.8-stable", 4, 8, 0, "stable"))
	if err := store.SaveSettings(dir, store.Settings{DefaultVersionID: v48.ID}); err != nil {
		t.Fatal(err)
	}

	pm := core.NewProjectManager(db, nil, nil, dir)
	add := func(name, declared string) project.Project {
		p, err := pm.AddProject(newProjectDir(t, dir, name, declared))
		if err != nil {
			t.Fatalf("AddProject(%s): %v", name, err)
		}
		return p
	}
	ready := add("ready", "4.2")
	missing := add("missing", "4.3")
	upgrading := add("upgrading", "4.2")
	if _, err := pm.SetVersionMode(upgrading.ID, project.ModeDefault, ""); err != nil {
		t.Fatal(err)
	}

	// A project reached through a symlink has to match the physical path
	// the launcher sees too.
	link := filepath.Join(dir, "link")
	if err := os.Symlink(newProjectDir(t, filepath.Join(dir, "real"), "linked", "4.8"), link); err != nil {
		t.Fatal(err)
	}
	linked, err := pm.AddProject(link)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := pm.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	table := pm.ShimTable(projects, "linux")

	byDir := map[string]shim.Target{}
	for _, target := range table.Projects {
		byDir[target.Dir] = target
	}
	if got := byDir[ready.Path]; got.Exec != v42.BinaryPath {
		t.Errorf("ready project runs %q, want %q (problem %q)", got.Exec, v42.BinaryPath, got.Problem)
	}
	if got := byDir[missing.Path]; got.Exec != "" || !strings.Contains(got.Problem, "isn't installed") {
		t.Errorf("missing project = %+v, want a problem naming the missing version", got)
	}
	if got := byDir[upgrading.Path]; got.Exec != "" || !strings.Contains(got.Problem, "upgrade") {
		t.Errorf("upgrading project = %+v, want it refused", got)
	}
	real, _ := filepath.EvalSymlinks(linked.Path)
	if byDir[linked.Path].Exec != v48.BinaryPath || byDir[real].Exec != v48.BinaryPath {
		t.Errorf("symlinked project: link entry %+v, physical entry %+v, want both to run 4.8", byDir[linked.Path], byDir[real])
	}
	if table.Default != v48.BinaryPath {
		t.Errorf("Default = %q, want %q", table.Default, v48.BinaryPath)
	}
}
