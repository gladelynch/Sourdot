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

// ProjectManager orchestrates project scanning, version resolution, and
// launching. UI-agnostic for the same reason as VersionManager.
//
// The governing rule for everything here: a project is never launched in
// an engine version it didn't ask for. Opening a project in a newer editor
// makes Godot rewrite its files to the new format, with no way back, so
// resolution only ever follows something the user or the project itself
// declared. When nothing declares a usable version, resolution reports
// what's needed and stops -- installing is a separate, explicit step (see
// InstallForProject).
type ProjectManager struct {
	db       *store.DB
	events   EventSink
	versions *VersionManager
	dataDir  string
}

// NewProjectManager constructs a ProjectManager. versions is used by
// InstallForProject to fetch a version a project needs. dataDir is where
// settings.json lives, read on demand to pick up the default version;
// passing "" simply disables the default-version mode (used by tests that
// have no settings file).
func NewProjectManager(db *store.DB, events EventSink, versions *VersionManager, dataDir string) *ProjectManager {
	return &ProjectManager{db: db, events: events, versions: versions, dataDir: dataDir}
}

// AddProject registers the Godot project found in dir. Fails if dir
// doesn't contain a project.godot.
//
// A new project starts in project.ModeProject: it sticks to the version
// recorded in its own project settings. Nothing is installed or launched
// here, so adding a project can never touch its files.
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
		DeclaredVersion: info.FeatureVersion,
		UsesCSharp:      info.UsesCSharp,
		VersionMode:     project.ModeProject,
		AddedAt:         time.Now(),
	}
	if err := pm.db.PutProject(proj); err != nil {
		return project.Project{}, err
	}
	return proj, nil
}

// ListProjects returns every tracked project, with Missing,
// DeclaredVersion, Resolution, and Thumbnail recomputed against the
// current filesystem state (not persisted -- recomputing on every read
// means a project that's moved back into place stops being flagged
// automatically rather than sticking at a stale "missing", and a version
// bump made in the editor itself shows up without a remove/re-add).
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
// project.Project) from the current state of disk.
func (pm *ProjectManager) enrichLive(p *project.Project) {
	icon := pm.refreshFromDisk(p)
	res := pm.ResolveVersion(*p)
	p.Resolution = &res
	p.Thumbnail = project.ResolveThumbnail(p.Path, icon)
}

// refreshFromDisk re-reads project.godot into p's version/C# fields and
// returns its config/icon value. Best-effort: leaves p untouched if the
// file can't be found or read.
//
// Resolution runs off these fields, so they have to come from disk rather
// than the stored record every time -- a project upgraded in the editor
// outside Sourdot declares a new version, and a stale copy of the old one
// is exactly the kind of thing that would send it back into the wrong
// editor.
func (pm *ProjectManager) refreshFromDisk(p *project.Project) (icon string) {
	projectFile, err := project.FindProjectFile(p.Path)
	if err != nil {
		return ""
	}
	info, err := project.ParseProjectGodot(projectFile)
	if err != nil {
		return ""
	}
	p.DeclaredVersion = info.FeatureVersion
	p.DetectedVersion = project.MajorFromConfigVersion(info.ConfigVersion)
	p.UsesCSharp = info.UsesCSharp
	return info.Icon
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

// SetVersionMode switches how a project picks its editor. versionID is
// required for project.ModePinned and ignored otherwise -- it's cleared
// rather than kept, so a project switched back to ModeProject can't
// silently resurrect an old pin later.
func (pm *ProjectManager) SetVersionMode(id, mode, versionID string) (project.Project, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return project.Project{}, err
	}
	if !ok {
		return project.Project{}, fmt.Errorf("project %s not found", id)
	}

	switch mode {
	case project.ModeProject, project.ModeDefault:
		proj.VersionMode = mode
		proj.PinnedVersionID = ""
	case project.ModePinned:
		if versionID == "" {
			return project.Project{}, fmt.Errorf("pinning %q needs a version to pin to", proj.Name)
		}
		if _, found, err := pm.db.GetInstalledVersion(versionID); err != nil {
			return project.Project{}, err
		} else if !found {
			return project.Project{}, fmt.Errorf("version %s is not installed", versionID)
		}
		proj.VersionMode = mode
		proj.PinnedVersionID = versionID
	default:
		return project.Project{}, fmt.Errorf("unknown version mode %q", mode)
	}

	return proj, pm.db.PutProject(proj)
}

// OpenProject launches a project in the editor its resolution names.
//
// It never installs anything and never substitutes a different build: a
// project whose version isn't installed, or that declares no version at
// all, returns an error saying so and launches nothing. Installing the
// missing version is the caller's explicit next step (InstallForProject),
// so the user always gets the chance to cancel instead of having an engine
// upgrade happen on their behalf.
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
	pm.refreshFromDisk(&proj)

	res := pm.ResolveVersion(proj)
	switch res.Status {
	case project.StatusUnknown:
		return install.InstalledVersion{}, nil, fmt.Errorf("%q doesn't record which Godot version it uses -- choose one for it before opening", proj.Name)
	case project.StatusMissing:
		return install.InstalledVersion{}, nil, fmt.Errorf("%q needs Godot %s, which isn't installed", proj.Name, res.Label)
	}

	iv, found, err := pm.db.GetInstalledVersion(res.VersionID)
	if err != nil {
		return install.InstalledVersion{}, nil, err
	}
	if !found {
		// Removed between resolving and launching. Refuse rather than
		// reach for the nearest sibling.
		return install.InstalledVersion{}, nil, fmt.Errorf("Godot %s is no longer installed", res.Label)
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

// InstallForProject downloads and installs the version a project needs,
// for the case where its resolution is project.StatusMissing. It is only
// ever reached from a deliberate user action, which is the point: the
// engine a project opens in is never changed as a side effect of opening
// it.
func (pm *ProjectManager) InstallForProject(ctx context.Context, id string) (install.InstalledVersion, error) {
	proj, ok, err := pm.db.GetProject(id)
	if err != nil {
		return install.InstalledVersion{}, err
	}
	if !ok {
		return install.InstalledVersion{}, fmt.Errorf("project %s not found", id)
	}
	pm.refreshFromDisk(&proj)

	spec, _, ok := pm.resolveTarget(proj)
	if !ok {
		return install.InstalledVersion{}, fmt.Errorf("%q doesn't record which Godot version it uses -- choose one for it first", proj.Name)
	}

	pm.emit(Event{Type: "project_install_started", ID: proj.ID, Data: map[string]any{
		"version": spec.Version,
		"isMono":  spec.IsMono,
	}})
	iv, err := pm.installSpec(ctx, spec)
	if err != nil {
		return install.InstalledVersion{}, fmt.Errorf("installing Godot %s: %w", spec.Version, err)
	}
	return iv, nil
}

// ResolveVersion reports which editor a project would launch with, and
// whether it's actually installed, without launching or installing
// anything. The Projects view renders this directly, so "needs Godot 4.3"
// is visible in the row rather than being discovered by clicking Open.
func (pm *ProjectManager) ResolveVersion(proj project.Project) project.VersionResolution {
	mode := project.ResolveMode(proj)
	res := project.VersionResolution{Mode: mode, Status: project.StatusUnknown}

	spec, source, ok := pm.resolveTarget(proj)

	// A pin whose build has since been uninstalled falls through to what
	// the project declares (so the row still says something useful), but
	// the user chose that pin and needs to know it's gone.
	dangling := ""
	if mode == project.ModePinned && source != "pinned version" {
		dangling = "the version this project was pinned to is no longer installed"
	}

	if !ok {
		res.Source = source
		res.Warning = firstNonEmpty(dangling, unresolvedWarning(mode, proj))
		return res
	}

	res.Source = source
	res.Version = spec.Version
	res.IsMono = spec.IsMono
	res.Label = monoLabel(targetLabel(spec), spec.IsMono)

	iv, found, err := pm.findInstalled(spec)
	if err != nil || !found {
		res.Status = project.StatusMissing
		res.Warning = dangling
		return res
	}

	res.Status = project.StatusReady
	res.VersionID = iv.ID
	res.IsMono = iv.IsMono
	res.Version = iv.Version
	res.Label = monoLabel(iv.TagOrReconstructed(), iv.IsMono)
	res.Warning = firstNonEmpty(dangling, mismatchWarning(iv, proj))
	res.WillUpgrade = willUpgrade(iv, proj)
	return res
}

// willUpgrade reports whether launching iv would make Godot rewrite the
// project to a newer format -- the one outcome the user can't undo, so the
// UI confirms before it happens.
func willUpgrade(iv install.InstalledVersion, proj project.Project) bool {
	major, minor, ok := project.MajorMinor(proj.DeclaredVersion)
	if !ok {
		return false
	}
	return iv.Major > major || (iv.Major == major && iv.Minor > minor)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveTarget picks the version a project should use, without consulting
// what's installed. Precedence depends on the project's mode:
//
//	ModePinned  -- the exact build the user pinned.
//	ModeDefault -- the default version from Settings, whatever it is now.
//	ModeProject -- a pin file in the project folder (.sourdot-version, then
//	               the incumbent .godot-version and .tool-versions), else
//	               the version project.godot itself declares.
//
// ModePinned and ModeDefault outrank the in-repo pin files because both
// are a direct, per-project choice made in this app on this machine; the
// old blanket "unpinned projects use the default" tier is gone, since that
// was one of the two ways a project could end up in an engine it never
// asked for.
//
// ok is false when nothing declares a usable version. There is no
// fall-through to "newest installed build of the same major": a major
// version is not a version, and picking the newest build of it is exactly
// how a 4.2 project ends up irreversibly upgraded by a 4.8 editor.
func (pm *ProjectManager) resolveTarget(proj project.Project) (spec project.PinSpec, source string, ok bool) {
	switch project.ResolveMode(proj) {
	case project.ModePinned:
		if iv, found, _ := pm.db.GetInstalledVersion(proj.PinnedVersionID); found {
			return specFor(iv), "pinned version", true
		}
		// The pinned build has since been removed. Fall through to what
		// the project declares so the row can still say what it needs;
		// ResolveVersion flags the dangling pin separately.
		return pm.declaredTarget(proj)

	case project.ModeDefault:
		if iv, found := pm.defaultVersion(); found {
			return specFor(iv), "default version", true
		}
		return project.PinSpec{}, "default version", false

	default:
		return pm.declaredTarget(proj)
	}
}

// declaredTarget resolves the version the project itself declares: an
// in-repo pin file first (an explicit, checked-in statement, and precise
// enough to name a patch), then project.godot's config/features.
func (pm *ProjectManager) declaredTarget(proj project.Project) (project.PinSpec, string, bool) {
	if spec, ok := project.ReadPinFile(proj.Path); ok {
		spec.IsMono = spec.IsMono || proj.UsesCSharp
		return spec, ".sourdot-version", true
	}
	if spec, ok := project.ReadGodotVersionFile(proj.Path); ok {
		spec.IsMono = spec.IsMono || proj.UsesCSharp
		return spec, ".godot-version", true
	}
	if spec, ok := project.ReadToolVersionsFile(proj.Path); ok {
		spec.IsMono = spec.IsMono || proj.UsesCSharp
		return spec, ".tool-versions", true
	}
	if spec, ok := project.SeriesSpec(proj.DeclaredVersion, proj.UsesCSharp); ok {
		return spec, "project.godot", true
	}
	return project.PinSpec{}, "project.godot", false
}

// unresolvedWarning explains why a project has no version to launch with,
// in terms of what the user can do about it.
func unresolvedWarning(mode string, proj project.Project) string {
	if mode == project.ModeDefault {
		return "no default version is set -- choose one in Settings"
	}
	if proj.DetectedVersion == "3" {
		// Godot 3 writes no config/features line at all, so there is
		// genuinely nothing in the project to read a version off.
		return "Godot 3 projects don't record their exact version -- choose one for this project"
	}
	return "this project doesn't record which Godot version it uses -- choose one for it"
}

// mismatchWarning flags a resolved build that will work but may not be
// what the user expects: a different series than the project declares
// (Godot will upgrade the project files on open, one-way), or a non-.NET
// build for a C# project (which simply won't build).
func mismatchWarning(iv install.InstalledVersion, proj project.Project) string {
	if proj.UsesCSharp && !iv.IsMono {
		return "this project uses C# but this build has no .NET support"
	}
	major, minor, ok := project.MajorMinor(proj.DeclaredVersion)
	if !ok || (iv.Major == major && iv.Minor == minor) {
		return ""
	}
	if iv.Major > major || (iv.Major == major && iv.Minor > minor) {
		return fmt.Sprintf("this project was last saved in Godot %s -- opening it in %d.%d will upgrade its files, and that can't be undone", proj.DeclaredVersion, iv.Major, iv.Minor)
	}
	return fmt.Sprintf("this project was last saved in Godot %s -- %d.%d is older and may not open it correctly", proj.DeclaredVersion, iv.Major, iv.Minor)
}

// monoLabel renders a version for display, marking .NET builds.
func monoLabel(version string, isMono bool) string {
	if isMono {
		return version + " (.NET)"
	}
	return version
}

// targetLabel names a version target that hasn't been matched to an
// installed build yet -- so, unlike a matched one, it has no release tag to
// show.
//
// A MinorOnly target renders as a series ("4.8.x"), never as a bare "4.8".
// project.godot records only the major.minor, so "4.8" there resolves to
// whichever of 4.8-dev3, 4.8-beta1, 4.8-rc1 or 4.8-stable is current, and
// those are four different builds -- a bare "4.8" in the UI would read as
// an exact version and hide that. Everywhere a specific build *is* known,
// callers show its full release tag instead; nothing in the UI ever
// displays a version number with its build label stripped off.
func targetLabel(spec project.PinSpec) string {
	if spec.TagName != "" {
		return spec.TagName
	}
	if spec.MinorOnly {
		return spec.Version + ".x"
	}
	return spec.Version
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
		return install.InstalledVersion{}, false // default points at a version since removed
	}
	return iv, true
}

// findInstalled locates the installed build a spec refers to. Three kinds
// of spec, narrowest first:
//
//   - A tagged spec must match that exact build -- a version-number match
//     isn't good enough, since 4.8-dev2 and 4.8-dev3 are both version 4.8.0.
//   - A MinorOnly spec (read from config/features, which records no more
//     than "4.3") matches any build in that series, taking the best one by
//     seriesPrefers: the most recent 4.3, which is stable once 4.3 has
//     shipped and the latest pre-release before that.
//   - A plain version spec (a hand-written pin file) matches any build of
//     that exact version, likewise, so the answer doesn't depend on the
//     order the store hands records back in.
//
// A .NET spec only ever matches a .NET build, since a standard build can't
// compile C# at all. A standard spec prefers a standard build but will
// accept a .NET one from the same series -- .NET builds run GDScript
// projects perfectly well, and refusing would mean reporting "not
// installed" for a version sitting right there.
func (pm *ProjectManager) findInstalled(spec project.PinSpec) (install.InstalledVersion, bool, error) {
	installed, err := pm.db.ListInstalledVersions()
	if err != nil {
		return install.InstalledVersion{}, false, err
	}

	var match, fallback install.InstalledVersion
	haveMatch, haveFallback := false, false
	for _, iv := range installed {
		if !matchesVersion(iv, spec) {
			continue
		}
		if iv.IsMono == spec.IsMono {
			if !haveMatch || seriesPrefers(iv.Patch, iv.Label, match.Patch, match.Label) {
				match, haveMatch = iv, true
			}
			continue
		}
		if !spec.IsMono {
			if !haveFallback || seriesPrefers(iv.Patch, iv.Label, fallback.Patch, fallback.Label) {
				fallback, haveFallback = iv, true
			}
		}
	}
	if haveMatch {
		return match, true, nil
	}
	return fallback, haveFallback, nil
}

// matchesVersion reports whether iv satisfies spec's version constraint,
// ignoring the .NET variant (which findInstalled handles).
func matchesVersion(iv install.InstalledVersion, spec project.PinSpec) bool {
	if spec.TagName != "" {
		return iv.TagOrReconstructed() == spec.TagName
	}
	if spec.MinorOnly {
		major, minor, ok := project.MajorMinor(spec.Version)
		return ok && iv.Major == major && iv.Minor == minor
	}
	return sameVersion(iv.Version, spec.Version)
}

// installSpec finds the release a spec names and installs it via
// VersionManager, reusing the exact same download/verify/extract pipeline
// the Versions view uses. A tagged spec installs that exact build; a plain
// version spec resolves to that version's stable release, matched on the
// catalog's series rather than by rebuilding a tag string (Godot writes
// "4.4-stable", never "4.4.0-stable", so either spelling of the version in
// a pin file has to work); a MinorOnly spec installs the most recent build
// in the series it names -- see pickSeriesRelease.
func (pm *ProjectManager) installSpec(ctx context.Context, spec project.PinSpec) (install.InstalledVersion, error) {
	releases, err := pm.versions.ListAvailable(ctx)
	if err != nil {
		return install.InstalledVersion{}, err
	}

	if spec.MinorOnly {
		major, minor, ok := project.MajorMinor(spec.Version)
		if !ok {
			return install.InstalledVersion{}, fmt.Errorf("%q isn't a Godot version series", spec.Version)
		}
		rel, found := pickSeriesRelease(releases, major, minor, spec.IsMono)
		if !found {
			return install.InstalledVersion{}, fmt.Errorf("no Godot %s release is available for this system", spec.Version)
		}
		return pm.versions.InstallVersion(ctx, rel, spec.IsMono)
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

// seriesPrefers ranks two builds of the same major.minor for a series
// target -- what a project means when it records "4.8" and nothing more.
//
// "4.8" means the most recent 4.8: once the series has shipped, that's
// 4.8-stable and then its patch releases; until it has, it's the latest
// pre-release, because that is the only 4.8 that exists. So:
//
//  1. A stable build beats a pre-release outright, whatever the numbers.
//     Otherwise a project on 4.8-stable would jump to a 4.8.1 release
//     candidate the moment one was published.
//  2. Between two builds of the same stability, the newest patch wins.
//  3. Between builds of the same patch, the later stage wins -- rc1 over
//     beta2 over dev5 -- which is also their release order.
func seriesPrefers(aPatch int, aLabel string, bPatch int, bLabel string) bool {
	if aStable, bStable := isStableLabel(aLabel), isStableLabel(bLabel); aStable != bStable {
		return aStable
	}
	if aPatch != bPatch {
		return aPatch > bPatch
	}
	return release.CompareLabels(aLabel, bLabel) > 0
}

func isStableLabel(label string) bool {
	channel, _, _ := release.ClassifyLabel(label)
	return channel == release.ChannelStable
}

// pickSeriesRelease chooses which build in a series to install, from the
// full catalog. Pre-releases are candidates rather than filtered out: a
// project declaring "4.8" while 4.8 is still in beta would otherwise have
// nothing installable at all, which is a dead end, not a safeguard.
//
// Candidates are limited to releases that actually ship an asset for this
// host and variant, so the newest build in a series can't be picked and
// then fail at download time while an older one that would have worked
// sits right behind it.
func pickSeriesRelease(releases []release.Release, major, minor int, isMono bool) (release.Release, bool) {
	var best release.Release
	found := false
	for _, rel := range releases {
		if rel.Major != major || rel.Minor != minor {
			continue
		}
		if _, err := release.FindHostAsset(rel, isMono); err != nil {
			continue
		}
		if !found || seriesPrefers(rel.Patch, rel.Label, best.Patch, best.Label) {
			best, found = rel, true
		}
	}
	return best, found
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
