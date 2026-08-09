//go:build integration

package core_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gladelynch/sourdot/internal/core"
	"github.com/gladelynch/sourdot/internal/godot/release"
	"github.com/gladelynch/sourdot/internal/store"
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

// TestReleaseJSONRoundTrip_ThenInstall exercises the specific risk in M2's
// App.InstallVersion(rel release.Release, isMono bool) binding: the
// frontend never constructs a Release itself, it round-trips one it
// received from ListAvailableVersions through the JS object Wails hands
// back on the next call. This reproduces that exact
// marshal-JSON/unmarshal-JSON cycle (standing in for the frontend's
// receive-then-send-back trip) without needing a GUI click, and checks no
// field is lost -- particularly Assets and ChecksumsURL, which
// InstallVersion depends on -- then performs a real install with the
// round-tripped value to prove it still works end to end.
func TestReleaseJSONRoundTrip_ThenInstall(t *testing.T) {
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
	original := releases[0]

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTripped release.Release
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, roundTripped) {
		t.Fatalf("release lost data across JSON round-trip:\noriginal:      %+v\nround-tripped: %+v", original, roundTripped)
	}
	if len(roundTripped.Assets) == 0 {
		t.Fatal("round-tripped release has no Assets -- InstallVersion would fail to find a host asset")
	}

	iv, err := vm.InstallVersion(ctx, roundTripped, false)
	if err != nil {
		t.Fatalf("InstallVersion with round-tripped release: %v", err)
	}
	if _, err := os.Stat(iv.BinaryPath); err != nil {
		t.Fatalf("binary not found at %s: %v", iv.BinaryPath, err)
	}

	if err := vm.RemoveVersion(iv.ID); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
}
