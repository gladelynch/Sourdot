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
		t.Fatal("expected a non-empty release catalog")
	}

	rel := newestStable(t, releases)
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
		t.Fatal("expected a non-empty release catalog")
	}

	rel := newestStable(t, releases)
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

// newestStable returns the newest stable release in a catalog that now
// leads with dev snapshots. The install tests deliberately exercise a
// stable build: it's the variant with the most consistent asset layout,
// so a failure points at our code rather than at an in-flux snapshot.
func newestStable(t *testing.T, releases []release.Release) release.Release {
	t.Helper()
	for _, rel := range releases {
		if rel.Channel == release.ChannelStable {
			return rel
		}
	}
	t.Fatal("no stable release found in catalog")
	return release.Release{}
}

// TestPrereleaseChannelsPresent guards the whole point of sourcing from
// godot-builds: the catalog has to carry pre-releases, not just the stable
// tags godotengine/godot exposes. It also checks every entry came out of
// parsing with the derived fields the UI groups and links on.
func TestPrereleaseChannelsPresent(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	vm := core.NewVersionManager(db, nil, t.TempDir(), "")
	releases, err := vm.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}

	counts := map[string]int{}
	for _, rel := range releases {
		counts[rel.Channel]++

		if rel.Series == "" {
			t.Errorf("%s has no Series, so it can't be grouped", rel.TagName)
		}
		if rel.ChangelogURL == "" {
			t.Errorf("%s has no ChangelogURL", rel.TagName)
		}
		if rel.Major < 3 {
			t.Errorf("%s should have been filtered out (pre-3.x)", rel.TagName)
		}
	}
	t.Logf("catalog: %d releases, channels: %v", len(releases), counts)

	// Every channel below has dozens of real tags upstream; zero of any of
	// them means the source or the tag parser regressed.
	for _, channel := range []string{
		release.ChannelStable, release.ChannelRC, release.ChannelBeta, release.ChannelDev,
	} {
		if counts[channel] == 0 {
			t.Errorf("catalog contains no %s releases", channel)
		}
	}
}

// TestInstallByTag_FromTrimmedCatalog exercises the exact path the
// Versions page takes. The frontend only ever holds the trimmed catalog
// (no body, host assets only) and installs by passing a tag string back,
// so the risk worth testing is whether a tag taken from that trimmed view
// still resolves to a complete, installable Release on the backend --
// including the Assets and ChecksumsURL the trim strips down.
func TestInstallByTag_FromTrimmedCatalog(t *testing.T) {
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

	groups, err := vm.ListAvailableGrouped(ctx)
	if err != nil {
		t.Fatalf("ListAvailableGrouped: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected a non-empty grouped catalog")
	}

	// Pick a tag the way the UI would: walk the rendered groups for a
	// stable entry, then send only its tag string across the boundary.
	var tagName string
	for _, g := range groups {
		for _, rel := range g.Releases {
			if rel.Channel == release.ChannelStable {
				tagName = rel.TagName
				break
			}
		}
		if tagName != "" {
			break
		}
	}
	if tagName == "" {
		t.Fatal("no stable release in the grouped catalog")
	}
	t.Logf("installing by tag %q", tagName)

	resolved, err := vm.FindRelease(ctx, tagName)
	if err != nil {
		t.Fatalf("FindRelease(%q): %v", tagName, err)
	}
	if len(resolved.Assets) == 0 {
		t.Fatal("resolved release has no Assets -- InstallVersion would fail to find a host asset")
	}
	if resolved.ChecksumsURL == "" {
		t.Error("resolved release has no ChecksumsURL, so the install would skip verification")
	}

	iv, err := vm.InstallVersion(ctx, resolved, false)
	if err != nil {
		t.Fatalf("InstallVersion for tag %q: %v", tagName, err)
	}
	if iv.TagName != tagName {
		t.Errorf("installed record TagName = %q, want %q", iv.TagName, tagName)
	}
	if _, err := os.Stat(iv.BinaryPath); err != nil {
		t.Fatalf("binary not found at %s: %v", iv.BinaryPath, err)
	}

	if err := vm.RemoveVersion(iv.ID); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
}
