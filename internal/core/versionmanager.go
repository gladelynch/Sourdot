package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/godot/release"
	"github.com/gladelynch/sourdot/internal/store"
)

// VersionManager orchestrates Godot version discovery, install, and
// removal. It is UI-agnostic by design: app.go's Wails App struct is a
// thin adapter over this, and a future CLI/TUI (backlog) would bind
// directly to it instead of duplicating the logic.
type VersionManager struct {
	db          *store.DB
	events      EventSink
	releases    *release.Client
	versionsDir string
}

// NewVersionManager constructs a VersionManager. githubToken may be empty
// (unauthenticated GitHub API access, capped at 60 req/hr).
func NewVersionManager(db *store.DB, events EventSink, versionsDir, githubToken string) *VersionManager {
	return &VersionManager{
		db:          db,
		events:      events,
		releases:    release.NewClient(githubToken, db),
		versionsDir: versionsDir,
	}
}

// UpdateGitHubToken swaps in a new GitHub API client using token, so a
// token saved in Settings takes effect immediately rather than requiring
// an app restart.
func (vm *VersionManager) UpdateGitHubToken(token string) {
	vm.releases = release.NewClient(token, vm.db)
}

// releaseIndexTTL bounds how stale the locally cached catalog may get
// before a page load refetches. Published releases never change, so the
// only thing a refetch can discover is a brand-new one -- six hours keeps
// the page instant on every launch while still surfacing a fresh beta the
// same day. The Refresh button bypasses this entirely.
const releaseIndexTTL = 6 * time.Hour

// ListAvailable returns the full Godot release catalog -- every channel,
// stable and pre-release -- serving the cached index when it's fresh and
// refetching when it isn't.
func (vm *VersionManager) ListAvailable(ctx context.Context) ([]release.Release, error) {
	if cached, fetchedAt, ok := vm.cachedIndex(); ok && time.Since(fetchedAt) < releaseIndexTTL {
		return cached, nil
	}
	return vm.RefreshAvailable(ctx)
}

// RefreshAvailable refetches the catalog from GitHub unconditionally,
// falling back to the cached index if the network or the rate limit says
// no -- a stale catalog is far more useful here than an error page, since
// almost every entry in it is historical anyway.
func (vm *VersionManager) RefreshAvailable(ctx context.Context) ([]release.Release, error) {
	releases, err := vm.releases.ListReleases(ctx)
	if err != nil {
		if cached, _, ok := vm.cachedIndex(); ok {
			return cached, nil
		}
		return nil, err
	}
	if raw, mErr := json.Marshal(releases); mErr == nil {
		_ = vm.db.PutReleaseIndex(raw) // best-effort: a cache write failure shouldn't fail the fetch
	}
	return releases, nil
}

// ListAvailableGrouped returns the catalog trimmed to this host's
// installable assets and chunked into per-series groups, ready for the
// Versions page to render directly.
func (vm *VersionManager) ListAvailableGrouped(ctx context.Context) ([]release.SeriesGroup, error) {
	releases, err := vm.ListAvailable(ctx)
	if err != nil {
		return nil, err
	}
	return release.GroupBySeries(release.ForCatalog(releases)), nil
}

// RefreshAvailableGrouped is ListAvailableGrouped with a forced refetch.
func (vm *VersionManager) RefreshAvailableGrouped(ctx context.Context) ([]release.SeriesGroup, error) {
	releases, err := vm.RefreshAvailable(ctx)
	if err != nil {
		return nil, err
	}
	return release.GroupBySeries(release.ForCatalog(releases)), nil
}

// FindRelease resolves a release tag to the full Release record, including
// the assets and checksum URL that InstallVersion needs.
//
// The frontend installs by tag rather than by handing back a Release
// object it received earlier: the catalog it holds is deliberately trimmed
// (no body, host assets only), and re-resolving here means an install
// always runs against complete, backend-owned data.
func (vm *VersionManager) FindRelease(ctx context.Context, tagName string) (release.Release, error) {
	releases, err := vm.ListAvailable(ctx)
	if err != nil {
		return release.Release{}, err
	}
	for _, rel := range releases {
		if rel.TagName == tagName {
			return rel, nil
		}
	}
	return release.Release{}, fmt.Errorf("release %s not found in the available catalog", tagName)
}

func (vm *VersionManager) cachedIndex() ([]release.Release, time.Time, bool) {
	raw, fetchedAt, ok := vm.db.GetReleaseIndex()
	if !ok {
		return nil, time.Time{}, false
	}
	var releases []release.Release
	if err := json.Unmarshal(raw, &releases); err != nil || len(releases) == 0 {
		return nil, time.Time{}, false
	}
	return releases, fetchedAt, true
}

// ListInstalled returns every version currently installed on disk, newest
// and most stable first (see sortsBefore), with TagName backfilled on
// records written before that field existed -- the Versions page matches
// installed versions to catalog entries by tag, so it must never see an
// empty one.
//
// The ordering is applied here rather than left to each caller because the
// store hands records back in key order, which is install order for
// practical purposes -- meaningless to a user reading a version list.
func (vm *VersionManager) ListInstalled() ([]install.InstalledVersion, error) {
	installed, err := vm.db.ListInstalledVersions()
	if err != nil {
		return nil, err
	}
	for i := range installed {
		installed[i].TagName = installed[i].TagOrReconstructed()
	}
	sortInstalled(installed)
	return installed, nil
}

// InstallVersion downloads, verifies, and extracts the release asset
// matching the current host platform and the requested standard/mono
// variant, emitting download_progress/checksum_verified/install_complete
// events as it goes, and persists the resulting InstalledVersion.
func (vm *VersionManager) InstallVersion(ctx context.Context, rel release.Release, isMono bool) (install.InstalledVersion, error) {
	asset, err := release.FindHostAsset(rel, isMono)
	if err != nil {
		return install.InstalledVersion{}, err
	}

	id := install.SyntheticID(rel.TagName, asset.OS, asset.Arch, isMono)
	finalDir := install.InstallDir(vm.versionsDir, id)
	if _, err := os.Stat(finalDir); err == nil {
		return install.InstalledVersion{}, fmt.Errorf("version %s is already installed", id)
	}

	tmpDir, err := os.MkdirTemp(vm.versionsDir, ".download-*")
	if err != nil {
		return install.InstalledVersion{}, err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, asset.Name)
	err = install.Download(ctx, asset.DownloadURL, zipPath, func(downloaded, total int64) {
		vm.emit(Event{Type: "download_progress", ID: id, Data: map[string]any{
			"downloaded": downloaded,
			"total":      total,
		}})
	})
	if err != nil {
		return install.InstalledVersion{}, fmt.Errorf("downloading %s: %w", asset.Name, err)
	}

	verified, err := install.VerifyChecksum(ctx, rel.ChecksumsURL, asset.Name, zipPath)
	if err != nil {
		return install.InstalledVersion{}, fmt.Errorf("checksum verification failed for %s: %w", asset.Name, err)
	}
	vm.emit(Event{Type: "checksum_verified", ID: id, Data: map[string]any{"verified": verified}})

	stagingDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return install.InstalledVersion{}, err
	}
	if err := install.ExtractZip(zipPath, stagingDir); err != nil {
		return install.InstalledVersion{}, fmt.Errorf("extracting %s: %w", asset.Name, err)
	}

	if err := os.Rename(stagingDir, finalDir); err != nil {
		return install.InstalledVersion{}, fmt.Errorf("finalizing install: %w", err)
	}

	binaryPath, err := install.ResolveBinaryPath(finalDir, asset.OS)
	if err != nil {
		_ = os.RemoveAll(finalDir)
		return install.InstalledVersion{}, err
	}

	size, _ := install.DirSize(finalDir) // best-effort; a size of 0 isn't fatal

	iv := install.InstalledVersion{
		ID:          id,
		TagName:     rel.TagName,
		Version:     fmt.Sprintf("%d.%d.%d", rel.Major, rel.Minor, rel.Patch),
		Major:       rel.Major,
		Minor:       rel.Minor,
		Patch:       rel.Patch,
		Label:       rel.Label,
		IsMono:      isMono,
		OS:          asset.OS,
		Arch:        asset.Arch,
		InstallPath: finalDir,
		BinaryPath:  binaryPath,
		SourceURL:   asset.DownloadURL,
		InstalledAt: time.Now(),
		SizeBytes:   size,
	}
	if err := vm.db.PutInstalledVersion(iv); err != nil {
		// Without this the extracted tree survives an unrecorded install:
		// nothing lists it, nothing can uninstall it, and the Stat check
		// above rejects every later attempt to install the same version.
		_ = os.RemoveAll(finalDir)
		return install.InstalledVersion{}, err
	}

	vm.emit(Event{Type: "install_complete", ID: id})
	return iv, nil
}

// RemoveVersion deletes an installed version's on-disk files and its
// store record.
//
// It clears both the canonical directory for the ID and whatever path the
// record happened to be written with, because os.RemoveAll treats "this
// path doesn't exist" as success: a record pointing somewhere stale would
// otherwise delete nothing, drop the record anyway, and leave the real
// folder orphaned -- invisible in the UI, still eating disk, and enough to
// make a reinstall fail with "already installed". A missing record is
// handled the same way, so a half-removed version can always be cleaned up
// by pressing Uninstall again.
func (vm *VersionManager) RemoveVersion(id string) error {
	iv, ok, err := vm.db.GetInstalledVersion(id)
	if err != nil {
		return err
	}

	canonical := install.InstallDir(vm.versionsDir, id)
	targets := []string{canonical}
	if recorded := recordedInstallDir(iv, id); recorded != "" && recorded != canonical {
		targets = append(targets, recorded)
	}

	removedAny := false
	for _, dir := range targets {
		if _, statErr := os.Lstat(dir); statErr == nil {
			removedAny = true
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("removing %s: %w", dir, err)
		}
	}

	if !ok && !removedAny {
		return fmt.Errorf("version %s is not installed", id)
	}
	return vm.db.DeleteInstalledVersion(id)
}

// recordedInstallDir returns the install path stored on iv, but only when
// it still looks like a directory this app created for that exact ID.
// InstallDir always names the leaf after the synthetic ID, so requiring
// that keeps a blank or truncated field -- os.RemoveAll("") and
// os.RemoveAll("/home/you") are both perfectly happy to run -- from
// pointing the delete at something that isn't ours.
func recordedInstallDir(iv install.InstalledVersion, id string) string {
	if iv.InstallPath == "" || !filepath.IsAbs(iv.InstallPath) {
		return ""
	}
	dir := filepath.Clean(iv.InstallPath)
	if filepath.Base(dir) != id {
		return ""
	}
	return dir
}

func (vm *VersionManager) emit(evt Event) {
	if vm.events != nil {
		vm.events.Emit(evt)
	}
}
