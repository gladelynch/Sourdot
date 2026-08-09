package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/store"
)

// removeVersionFixture builds a VersionManager over a real BoltDB and a
// real versions directory, with one installed version whose on-disk tree
// lives at diskDir and whose stored InstallPath is recordedPath -- the two
// differ in exactly the cases this file is about.
func removeVersionFixture(t *testing.T, id, recordedPath string, writeRecord bool) (*core.VersionManager, *store.DB, string) {
	t.Helper()

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	versionsDir := filepath.Join(dir, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if writeRecord {
		iv := install.InstalledVersion{
			ID:          id,
			TagName:     "4.8-dev3",
			InstallPath: recordedPath,
			BinaryPath:  filepath.Join(recordedPath, "godot"),
		}
		if err := db.PutInstalledVersion(iv); err != nil {
			t.Fatalf("PutInstalledVersion: %v", err)
		}
	}

	return core.NewVersionManager(db, nil, versionsDir, ""), db, versionsDir
}

func writeInstallTree(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "godot"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestRemoveVersion_StaleInstallPath is the regression test for uninstalls
// that reported success and deleted nothing. The stored InstallPath points
// at a directory that no longer exists (a moved data dir, a launcher with
// its own XDG_CONFIG_HOME), and os.RemoveAll is perfectly happy to
// "remove" a path that isn't there -- so the record went away while the
// real folder stayed, invisible to the UI and blocking any reinstall.
func TestRemoveVersion_StaleInstallPath(t *testing.T) {
	const id = "4.8-dev3-mono-linux-x86_64"
	stale := filepath.Join(t.TempDir(), "old-config", "versions", id)

	vm, db, versionsDir := removeVersionFixture(t, id, stale, true)
	onDisk := filepath.Join(versionsDir, id)
	writeInstallTree(t, onDisk)

	if err := vm.RemoveVersion(id); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}

	if _, err := os.Stat(onDisk); !os.IsNotExist(err) {
		t.Errorf("install directory still present after uninstall: %v", err)
	}
	if _, ok, err := db.GetInstalledVersion(id); err != nil || ok {
		t.Errorf("record still present after uninstall (ok=%v, err=%v)", ok, err)
	}
}

// TestRemoveVersion_OrphanedDirNoRecord covers the other half of the same
// bug: once a record is gone but its folder isn't, pressing Uninstall
// again must clean up rather than refuse with "not installed" and leave
// the folder there forever.
func TestRemoveVersion_OrphanedDirNoRecord(t *testing.T) {
	const id = "4.8-dev3-mono-linux-x86_64"

	vm, _, versionsDir := removeVersionFixture(t, id, "", false)
	onDisk := filepath.Join(versionsDir, id)
	writeInstallTree(t, onDisk)

	if err := vm.RemoveVersion(id); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
	if _, err := os.Stat(onDisk); !os.IsNotExist(err) {
		t.Errorf("orphaned directory still present after uninstall: %v", err)
	}
}

// TestRemoveVersion_NotInstalled keeps the error path honest: with neither
// a record nor a directory there's genuinely nothing to remove, and the
// caller should hear about it instead of getting a silent success.
func TestRemoveVersion_NotInstalled(t *testing.T) {
	vm, _, _ := removeVersionFixture(t, "", "", false)

	if err := vm.RemoveVersion("4.8-dev3-mono-linux-x86_64"); err == nil {
		t.Error("expected an error removing a version that was never installed")
	}
}

// TestRemoveVersion_IgnoresUnrelatedInstallPath guards the blast radius of
// deleting the recorded path: a record whose InstallPath was corrupted (or
// blank, which os.RemoveAll also accepts without complaint) must not take
// an unrelated directory with it.
func TestRemoveVersion_IgnoresUnrelatedInstallPath(t *testing.T) {
	const id = "4.8-dev3-mono-linux-x86_64"

	bystanderParent := t.TempDir()
	bystander := filepath.Join(bystanderParent, "important")
	writeInstallTree(t, bystander)

	vm, _, versionsDir := removeVersionFixture(t, id, bystanderParent, true)
	writeInstallTree(t, filepath.Join(versionsDir, id))

	if err := vm.RemoveVersion(id); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
	if _, err := os.Stat(bystander); err != nil {
		t.Errorf("uninstall deleted an unrelated directory: %v", err)
	}
}
