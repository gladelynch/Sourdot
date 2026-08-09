package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gladelynch/godotvm/internal/godot/install"
	"github.com/gladelynch/godotvm/internal/platform"
	"github.com/gladelynch/godotvm/internal/project"
	"github.com/gladelynch/godotvm/internal/store"
)

// ProjectManager orchestrates project scanning, pin resolution, and the
// resolve-then-launch flow (find the pinned InstalledVersion, auto-install
// it via VersionManager if missing, then exec the Godot binary). UI-
// agnostic for the same reason as VersionManager.
type ProjectManager struct {
	db       *store.DB
	events   EventSink
	versions *VersionManager
}

// NewProjectManager constructs a ProjectManager. versions is used to
// auto-install a project's pinned version when it isn't already present.
func NewProjectManager(db *store.DB, events EventSink, versions *VersionManager) *ProjectManager {
	return &ProjectManager{db: db, events: events, versions: versions}
}

// AddProject registers the Godot project found in dir. Fails if dir
// doesn't contain a project.godot.
func (pm *ProjectManager) AddProject(dir string) (project.Project, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return project.Project{}, err
	}

	projectFile, err := project.FindProjectFile(absDir)
	if err != nil {
		return project.Project{}, err
	}

	info, err := project.ParseProjectGodot(projectFile)
	if err != nil {
		return project.Project{}, fmt.Errorf("reading %s: %w", projectFile, err)
	}

	name := info.Name
	if name == "" {
		name = filepath.Base(absDir)
	}

	proj := project.Project{
		ID:              hashPath(absDir),
		Path:            absDir,
		Name:            name,
		DetectedVersion: project.MajorFromConfigVersion(info.ConfigVersion),
		UsesCSharp:      info.UsesCSharp,
		AddedAt:         time.Now(),
	}
	if err := pm.db.PutProject(proj); err != nil {
		return project.Project{}, err
	}
	return proj, nil
}

// ListProjects returns every tracked project, with Missing recomputed
// against the current filesystem state (not persisted -- recomputing on
// every read means a project that's moved back into place stops being
// flagged automatically, rather than sticking at a stale "missing").
func (pm *ProjectManager) ListProjects() ([]project.Project, error) {
	projects, err := pm.db.ListProjects()
	if err != nil {
		return nil, err
	}
	for i := range projects {
		_, statErr := os.Stat(projects[i].Path)
		projects[i].Missing = statErr != nil
	}
	return projects, nil
}

// RemoveProject stops tracking a project. It never touches the project's
// files on disk.
func (pm *ProjectManager) RemoveProject(id string) error {
	return pm.db.DeleteProject(id)
}

// SetFavorite and SetTags mutate a tracked project's metadata directly,
// rather than routing every small edit through a generic "update project"
// call -- keeps the persisted-state surface simple and each write
// unambiguous, which matters given a competitor's "favorites reset on
// restart" bug is an explicit anti-goal (see the plan).
func (pm *ProjectManager) SetFavorite(id string, favorite bool) (project.Project, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return project.Project{}, err
	}
	if !ok {
		return project.Project{}, fmt.Errorf("project %s not found", id)
	}
	proj.Favorite = favorite
	return proj, pm.db.PutProject(proj)
}

func (pm *ProjectManager) SetTags(id string, tags []string) (project.Project, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return project.Project{}, err
	}
	if !ok {
		return project.Project{}, fmt.Errorf("project %s not found", id)
	}
	proj.Tags = tags
	return proj, pm.db.PutProject(proj)
}

// SetPinnedVersion sets (or, with an empty id, clears) a project's
// explicit UI pin -- the highest-precedence entry in the resolution
// chain, ahead of any pin file on disk.
func (pm *ProjectManager) SetPinnedVersion(id, versionID string) (project.Project, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return project.Project{}, err
	}
	if !ok {
		return project.Project{}, fmt.Errorf("project %s not found", id)
	}
	proj.PinnedVersionID = versionID
	return proj, pm.db.PutProject(proj)
}

// OpenProject resolves which installed version a project should use (see
// ResolveVersionSpec), auto-installing it in the background first if it
// isn't present yet, then launches the Godot editor for it.
//
// Returns the *exec.Cmd of the launched editor without waiting on it --
// callers that don't need to track the child process (the production
// Wails binding) can simply discard it.
func (pm *ProjectManager) OpenProject(ctx context.Context, id string) (install.InstalledVersion, *exec.Cmd, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return install.InstalledVersion{}, nil, err
	}
	if !ok {
		return install.InstalledVersion{}, nil, fmt.Errorf("project %s not found", id)
	}
	if _, statErr := os.Stat(proj.Path); statErr != nil {
		return install.InstalledVersion{}, nil, fmt.Errorf("project folder not found at %s (it may have been moved or deleted)", proj.Path)
	}

	spec, _, ok := pm.ResolveVersionSpec(proj)
	if !ok {
		return install.InstalledVersion{}, nil, fmt.Errorf("could not determine which Godot version to use for %q -- pin one explicitly", proj.Name)
	}

	iv, found, err := pm.findInstalled(spec)
	if err != nil {
		return install.InstalledVersion{}, nil, err
	}
	if !found {
		pm.emit(Event{Type: "project_auto_install_started", ID: proj.ID, Data: map[string]any{
			"version": spec.Version,
			"isMono":  spec.IsMono,
		}})
		iv, err = pm.autoInstall(ctx, spec)
		if err != nil {
			return install.InstalledVersion{}, nil, fmt.Errorf("auto-installing Godot %s: %w", spec.Version, err)
		}
	}

	cmd, err := platform.Launch(iv.BinaryPath, proj.Path)
	if err != nil {
		return install.InstalledVersion{}, nil, fmt.Errorf("launching editor: %w", err)
	}

	proj.LastOpenedAt = time.Now()
	if err := pm.db.PutProject(proj); err != nil {
		return iv, cmd, err // launch already succeeded; surface the persistence error without pretending the open failed
	}
	return iv, cmd, nil
}

// ResolveVersionSpec determines which version a project should launch
// with, following the precedence chain: explicit UI pin (store) >
// .godotvm-version > .godot-version > .tool-versions > best-effort
// project.godot detection. The best-effort tier only carries a major
// version (see project.MajorFromConfigVersion), which isn't precise enough
// to auto-install on its own -- it only resolves if a matching major is
// already installed (newest patch wins; mono preferred when the project
// uses C#), otherwise ok is false and the caller should ask the user to
// pin a version explicitly.
func (pm *ProjectManager) ResolveVersionSpec(proj project.Project) (spec project.PinSpec, source string, ok bool) {
	if proj.PinnedVersionID != "" {
		if iv, found, _ := pm.db.GetInstalledVersion(proj.PinnedVersionID); found {
			return project.PinSpec{Version: iv.Version, IsMono: iv.IsMono}, "explicit pin", true
		}
	}
	if spec, ok := project.ReadPinFile(proj.Path); ok {
		return spec, ".godotvm-version", true
	}
	if spec, ok := project.ReadGodotVersionFile(proj.Path); ok {
		return spec, ".godot-version", true
	}
	if spec, ok := project.ReadToolVersionsFile(proj.Path); ok {
		return spec, ".tool-versions", true
	}
	if proj.DetectedVersion != "" {
		if iv, found := pm.newestInstalledMajor(proj.DetectedVersion, proj.UsesCSharp); found {
			return project.PinSpec{Version: iv.Version, IsMono: iv.IsMono}, "detected version (newest installed match)", true
		}
	}
	return project.PinSpec{}, "", false
}

// newestInstalledMajor finds the newest installed version matching the
// given major ("3" or "4"), preferring a mono build when preferMono is
// set and one exists.
func (pm *ProjectManager) newestInstalledMajor(major string, preferMono bool) (install.InstalledVersion, bool) {
	installed, err := pm.db.ListInstalledVersions()
	if err != nil {
		return install.InstalledVersion{}, false
	}

	var best install.InstalledVersion
	found := false
	for _, iv := range installed {
		if fmt.Sprint(iv.Major) != major {
			continue
		}
		if preferMono && iv.IsMono != preferMono {
			continue
		}
		if !found || isNewer(iv, best) {
			best, found = iv, true
		}
	}
	if !found && preferMono {
		return pm.newestInstalledMajor(major, false) // fall back to standard build if no mono match exists
	}
	return best, found
}

func isNewer(a, b install.InstalledVersion) bool {
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	return a.Patch > b.Patch
}

func (pm *ProjectManager) findInstalled(spec project.PinSpec) (install.InstalledVersion, bool, error) {
	installed, err := pm.db.ListInstalledVersions()
	if err != nil {
		return install.InstalledVersion{}, false, err
	}
	for _, iv := range installed {
		if iv.Version == spec.Version && iv.IsMono == spec.IsMono {
			return iv, true, nil
		}
	}
	return install.InstalledVersion{}, false, nil
}

// autoInstall finds the stable release matching spec.Version and installs
// it via VersionManager, reusing the exact same download/verify/extract
// pipeline the Versions view uses.
func (pm *ProjectManager) autoInstall(ctx context.Context, spec project.PinSpec) (install.InstalledVersion, error) {
	releases, err := pm.versions.ListAvailable(ctx)
	if err != nil {
		return install.InstalledVersion{}, err
	}
	tag := spec.Version + "-stable"
	for _, rel := range releases {
		if rel.TagName == tag {
			return pm.versions.InstallVersion(ctx, rel, spec.IsMono)
		}
	}
	return install.InstalledVersion{}, fmt.Errorf("no stable release found matching %s", spec.Version)
}

func (pm *ProjectManager) emit(evt Event) {
	if pm.events != nil {
		pm.events.Emit(evt)
	}
}

// hashPath derives a stable Project.ID from an absolute path.
func hashPath(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:])[:16]
}
