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

	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/godot/release"
	"github.com/gladelynch/sourdot/internal/platform"
	"github.com/gladelynch/sourdot/internal/project"
	"github.com/gladelynch/sourdot/internal/store"
)

// ProjectManager orchestrates project scanning, pin resolution, and the
// resolve-then-launch flow (find the pinned InstalledVersion, auto-install
// it via VersionManager if missing, then exec the Godot binary). UI-
// agnostic for the same reason as VersionManager.
type ProjectManager struct {
	db       *store.DB
	events   EventSink
	versions *VersionManager
	dataDir  string
}

// NewProjectManager constructs a ProjectManager. versions is used to
// auto-install a project's pinned version when it isn't already present.
// dataDir is where settings.json lives, read on demand to pick up the
// default version; passing "" simply disables that tier of the resolution
// chain (used by tests that have no settings file).
func NewProjectManager(db *store.DB, events EventSink, versions *VersionManager, dataDir string) *ProjectManager {
	return &ProjectManager{db: db, events: events, versions: versions, dataDir: dataDir}
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

// ListProjects returns every tracked project, with Missing,
// DetectedVersionLabel, and Thumbnail recomputed against the current
// filesystem state (not persisted -- recomputing on every read means a
// project that's moved back into place stops being flagged automatically
// rather than sticking at a stale "missing", and an icon swap or engine
// upgrade shows up without a remove/re-add).
func (pm *ProjectManager) ListProjects() ([]project.Project, error) {
	projects, err := pm.db.ListProjects()
	if err != nil {
		return nil, err
	}
	for i := range projects {
		p := &projects[i]
		_, statErr := os.Stat(p.Path)
		p.Missing = statErr != nil
		if !p.Missing {
			pm.enrichLive(p)
		}
	}
	return projects, nil
}

// enrichLive fills in p's display-only fields (see their doc comments on
// project.Project) by re-parsing project.godot from disk. Best-effort:
// leaves them zero-valued if the file can't be found or read, same as if
// the project had no icon/parseable feature version.
func (pm *ProjectManager) enrichLive(p *project.Project) {
	projectFile, err := project.FindProjectFile(p.Path)
	if err != nil {
		return
	}
	info, err := project.ParseProjectGodot(projectFile)
	if err != nil {
		return
	}
	p.DetectedVersionLabel = info.FeatureVersion
	if p.DetectedVersionLabel == "" {
		p.DetectedVersionLabel = p.DetectedVersion
	}
	p.Thumbnail = project.ResolveThumbnail(p.Path, info.Icon)
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
// .sourdot-version > .godot-version > .tool-versions > the default version
// from Settings > best-effort project.godot detection. The best-effort
// tier only carries a major version (see project.MajorFromConfigVersion),
// which isn't precise enough to auto-install on its own -- it only
// resolves if a matching major is already installed (newest build wins;
// mono preferred when the project uses C#), otherwise ok is false and the
// caller should ask the user to pin a version explicitly.
func (pm *ProjectManager) ResolveVersionSpec(proj project.Project) (spec project.PinSpec, source string, ok bool) {
	if proj.PinnedVersionID != "" {
		if iv, found, _ := pm.db.GetInstalledVersion(proj.PinnedVersionID); found {
			return specFor(iv), "explicit pin", true
		}
	}
	if spec, ok := project.ReadPinFile(proj.Path); ok {
		return spec, ".sourdot-version", true
	}
	if spec, ok := project.ReadGodotVersionFile(proj.Path); ok {
		return spec, ".godot-version", true
	}
	if spec, ok := project.ReadToolVersionsFile(proj.Path); ok {
		return spec, ".tool-versions", true
	}
	if iv, found := pm.defaultVersion(); found && suitsProject(iv, proj) {
		return specFor(iv), "default version", true
	}
	if proj.DetectedVersion != "" {
		if iv, found := pm.newestInstalledMajor(proj.DetectedVersion, proj.UsesCSharp); found {
			return specFor(iv), "detected version (newest installed match)", true
		}
	}
	return project.PinSpec{}, "", false
}

// specFor builds a spec that names one exact installed build, so
// findInstalled can't substitute a sibling from the same series.
func specFor(iv install.InstalledVersion) project.PinSpec {
	return project.PinSpec{
		Version: iv.Version,
		IsMono:  iv.IsMono,
		TagName: iv.TagOrReconstructed(),
	}
}

// defaultVersion returns the version set as the default in Settings, if
// one is set and still installed. Settings are read on each call rather
// than cached at construction so changing the default in the Settings view
// takes effect on the next launch, not the next app restart.
func (pm *ProjectManager) defaultVersion() (install.InstalledVersion, bool) {
	if pm.dataDir == "" {
		return install.InstalledVersion{}, false
	}
	settings, err := store.LoadSettings(pm.dataDir)
	if err != nil || settings.DefaultVersionID == "" {
		return install.InstalledVersion{}, false
	}
	iv, found, err := pm.db.GetInstalledVersion(settings.DefaultVersionID)
	if err != nil || !found {
		return install.InstalledVersion{}, false // default points at a version since removed; fall through
	}
	return iv, true
}

// suitsProject guards the default-version tier against the two ways a
// blanket default would actively break a project: opening a Godot 3
// project in Godot 4 (an irreversible project-file upgrade prompt), and
// opening a C# project with a build that has no .NET support. In both
// cases resolution falls through to the detection tier, which picks a
// version that actually fits.
func suitsProject(iv install.InstalledVersion, proj project.Project) bool {
	if proj.DetectedVersion != "" && fmt.Sprint(iv.Major) != proj.DetectedVersion {
		return false
	}
	return !proj.UsesCSharp || iv.IsMono
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
		if !found || sortsBefore(iv, best) {
			best, found = iv, true
		}
	}
	if !found && preferMono {
		return pm.newestInstalledMajor(major, false) // fall back to standard build if no mono match exists
	}
	return best, found
}

// findInstalled locates the installed build a spec refers to. A spec
// carrying a tag must match that exact build -- a version-number match
// isn't good enough, since 4.8-dev2 and 4.8-dev3 are both version 4.8.0.
// A version-only spec (a hand-written pin file) matches any build of that
// version, taking the newest and most stable one so the answer doesn't
// depend on the order the store happens to hand records back in.
func (pm *ProjectManager) findInstalled(spec project.PinSpec) (install.InstalledVersion, bool, error) {
	installed, err := pm.db.ListInstalledVersions()
	if err != nil {
		return install.InstalledVersion{}, false, err
	}
	sortInstalled(installed)

	for _, iv := range installed {
		if iv.IsMono != spec.IsMono {
			continue
		}
		if spec.TagName != "" {
			if iv.TagOrReconstructed() == spec.TagName {
				return iv, true, nil
			}
			continue
		}
		if sameVersion(iv.Version, spec.Version) {
			return iv, true, nil
		}
	}
	return install.InstalledVersion{}, false, nil
}

// autoInstall finds the release a spec names and installs it via
// VersionManager, reusing the exact same download/verify/extract pipeline
// the Versions view uses. A spec carrying a tag installs that exact build;
// a version-only spec resolves to that version's stable release, matched
// on the catalog's series rather than by rebuilding a tag string (Godot
// writes "4.4-stable", never "4.4.0-stable", so either spelling of the
// version in a pin file has to work).
func (pm *ProjectManager) autoInstall(ctx context.Context, spec project.PinSpec) (install.InstalledVersion, error) {
	releases, err := pm.versions.ListAvailable(ctx)
	if err != nil {
		return install.InstalledVersion{}, err
	}
	for _, rel := range releases {
		if spec.TagName != "" {
			if rel.TagName == spec.TagName {
				return pm.versions.InstallVersion(ctx, rel, spec.IsMono)
			}
			continue
		}
		if rel.Channel == release.ChannelStable && sameVersion(rel.Series, spec.Version) {
			return pm.versions.InstallVersion(ctx, rel, spec.IsMono)
		}
	}
	if spec.TagName != "" {
		return install.InstalledVersion{}, fmt.Errorf("release %s is no longer available to download", spec.TagName)
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
