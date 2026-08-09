//go:build integration

package core_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gladelynch/godotvm/internal/core"
	"github.com/gladelynch/godotvm/internal/store"
)

// TestInstallVersion_EndToEnd exercises the real M1 pipeline against the
// live GitHub API and a real Godot download -- this is the M1 acceptance
// check from the plan ("install a real Godot version into the
// correctly-named versions directory and confirm the binary runs").
// Network-dependent by design and downloads tens of MB, so it's gated
// behind a build tag rather than running as part of the normal suite:
//
//	go test -tags integration ./internal/core/... -run EndToEnd -v
func TestInstallVersion_EndToEnd(t *testing.T) {
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

	ctx := context.Background()
	releases, err := vm.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("expected at least one stable release")
	}

	// GitHub returns releases newest first; install the newest.
	rel := releases[0]
	t.Logf("installing %s", rel.TagName)

	iv, err := vm.InstallVersion(ctx, rel, false)
	if err != nil {
		t.Fatalf("InstallVersion: %v", err)
	}
	t.Logf("installed %s -> %s", iv.ID, iv.BinaryPath)

	if _, err := os.Stat(iv.BinaryPath); err != nil {
		t.Fatalf("binary not found at %s: %v", iv.BinaryPath, err)
	}

	if runtime.GOOS != "windows" { // exec bit is meaningless to check on Windows
		out, err := exec.Command(iv.BinaryPath, "--version").CombinedOutput()
		if err != nil {
			t.Fatalf("running --version: %v\n%s", err, out)
		}
		t.Logf("--version output: %s", out)
	}

	installed, err := vm.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("ListInstalled: got %d records, want 1", len(installed))
	}

	if err := vm.RemoveVersion(iv.ID); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
	if _, err := os.Stat(iv.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("expected install dir removed, stat err = %v", err)
	}
}

// TestInstallVersion_Mono covers the folder-wrapped mono zip layout
// specifically (binary + GodotSharp/ support folder, with a binary name
// that doesn't match the folder name -- see layout.go's ResolveBinaryPath
// doc comment), which the standard-build test above doesn't exercise.
func TestInstallVersion_Mono(t *testing.T) {
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

	ctx := context.Background()
	releases, err := vm.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("expected at least one stable release")
	}

	rel := releases[0]
	t.Logf("installing %s (mono)", rel.TagName)

	iv, err := vm.InstallVersion(ctx, rel, true)
	if err != nil {
		t.Fatalf("InstallVersion: %v", err)
	}
	t.Logf("installed %s -> %s", iv.ID, iv.BinaryPath)

	if _, err := os.Stat(iv.BinaryPath); err != nil {
		t.Fatalf("binary not found at %s: %v", iv.BinaryPath, err)
	}
	if strings.Contains(iv.BinaryPath, "GodotSharp") {
		t.Fatalf("resolved binary path incorrectly points inside GodotSharp/: %s", iv.BinaryPath)
	}

	if runtime.GOOS != "windows" {
		out, err := exec.Command(iv.BinaryPath, "--version").CombinedOutput()
		if err != nil {
			t.Fatalf("running --version: %v\n%s", err, out)
		}
		t.Logf("--version output: %s", out)
	}

	if err := vm.RemoveVersion(iv.ID); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
}
