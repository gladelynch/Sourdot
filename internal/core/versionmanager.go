package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gladelynch/godotvm/internal/godot/install"
	"github.com/gladelynch/godotvm/internal/godot/release"
	"github.com/gladelynch/godotvm/internal/store"
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

// ListAvailable fetches stable Godot releases from GitHub.
func (vm *VersionManager) ListAvailable(ctx context.Context) ([]release.Release, error) {
	return vm.releases.ListStableReleases(ctx)
}

// ListInstalled returns every version currently installed on disk.
func (vm *VersionManager) ListInstalled() ([]install.InstalledVersion, error) {
	return vm.db.ListInstalledVersions()
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
		return install.InstalledVersion{}, err
	}

	vm.emit(Event{Type: "install_complete", ID: id})
	return iv, nil
}

// RemoveVersion deletes an installed version's on-disk files and its
// store record.
func (vm *VersionManager) RemoveVersion(id string) error {
	iv, ok, err := vm.db.GetInstalledVersion(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("version %s is not installed", id)
	}
	if err := os.RemoveAll(iv.InstallPath); err != nil {
		return err
	}
	return vm.db.DeleteInstalledVersion(id)
}

func (vm *VersionManager) emit(evt Event) {
	if vm.events != nil {
		vm.events.Emit(evt)
	}
}
