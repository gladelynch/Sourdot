// Package project scans folders for Godot projects, parses project.godot
// for engine-version/C# signals, and resolves per-project version pins
// (own format plus read-compat for incumbent formats).
package project

import "time"

// Version modes: how a project decides which editor to launch with. There
// is deliberately no "whatever's newest" mode -- opening a project in a
// newer editor triggers Godot's irreversible project-file upgrade, so the
// engine version is never guessed, only declared.
const (
	// ModeProject follows the version the project itself declares: a pin
	// file in the project folder, else project.godot's config/features.
	// The default for newly added projects.
	ModeProject = "project"

	// ModeDefault always uses the default version from Settings, whatever
	// it currently is. Opt-in per project.
	ModeDefault = "default"

	// ModePinned uses exactly the build named by Project.PinnedVersionID.
	ModePinned = "pinned"
)

// Resolution statuses. Only StatusReady may launch.
const (
	// StatusReady means the resolved version is installed and can launch.
	StatusReady = "ready"

	// StatusMissing means the project's version is known but not installed.
	// The UI names the version and offers to install it; it never
	// substitutes another build.
	StatusMissing = "missing"

	// StatusUnknown means nothing declares a version for this project --
	// typically a Godot 3 project, whose project.godot has no
	// config/features line at all. The user must choose one explicitly.
	StatusUnknown = "unknown"
)

// Project is a tracked Godot project folder, persisted in
// store.BucketProjects.
type Project struct {
	ID               string    `json:"id"`               // hash of the resolved absolute path
	Path             string    `json:"path"`             // absolute path to the project root (contains project.godot)
	Name             string    `json:"name"`             // from [application] config/name, falling back to the dir name
	DetectedVersion  string    `json:"detectedVersion"`  // major only ("3" or "4"), from project.godot's config_version -- see ParseProjectGodot; a sanity signal only, never enough to resolve a launch on its own
	DetectedRenderer string    `json:"detectedRenderer"` // best-effort; may be empty
	UsesCSharp       bool      `json:"usesCSharp"`       // [dotnet] section present, or a sibling .csproj
	VersionMode      string    `json:"versionMode"`      // one of the Mode* constants; "" on records written before this field existed -- read it through ResolveMode, never directly
	PinnedVersionID  string    `json:"pinnedVersionId"`  // references an install.InstalledVersion.ID; only consulted in ModePinned
	Favorite         bool      `json:"favorite"`
	Tags             []string  `json:"tags"`
	LastOpenedAt     time.Time `json:"lastOpenedAt"`
	AddedAt          time.Time `json:"addedAt"`
	Missing          bool      `json:"missing"` // path no longer exists on disk; surfaced, not silently dropped

	// DeclaredVersion, Resolution and Thumbnail are recomputed from disk on
	// every ProjectManager.ListProjects call (like Missing above), and
	// DeclaredVersion is refreshed again before anything resolves a launch.
	// Whatever ends up in the store is only ever a snapshot and is never
	// trusted: an engine upgrade made outside Sourdot changes the version a
	// project declares, and resolving against a stale copy of the old one
	// is precisely how a project would be sent back into the wrong editor.
	// Resolution and Thumbnail are zero-valued on a Project fetched any
	// other way (e.g. GetProject).
	DeclaredVersion string             `json:"declaredVersion"` // the major.minor the project declares in config/features, e.g. "4.3"; "" when it declares none (Godot 3 projects never do) -- see ParseProjectGodot
	Resolution      *VersionResolution `json:"resolution"`      // which editor this project would launch with, and whether it's installed
	Thumbnail       string             `json:"thumbnail"`       // data: URI of the project's icon, or "" if none found -- see ResolveThumbnail
}

// VersionResolution is the outcome of deciding which editor a project
// should launch with, in a form the UI can render directly. It is computed
// without launching anything, so a project that needs a version it doesn't
// have says so in its row rather than only failing at the moment the user
// clicks Open.
type VersionResolution struct {
	Mode   string `json:"mode"`   // the Mode* this was resolved under, defaulted -- see ResolveMode
	Status string `json:"status"` // one of the Status* constants
	Source string `json:"source"` // where the answer came from, for display: "project.godot", ".sourdot-version", "default version", "pinned version"

	// Version and Label describe the target. Version is the version number
	// ("4.3" from config/features, "4.3.1" from an exact pin); Label is its
	// display name -- the full release tag when the build is installed
	// ("4.3.1-stable"), the bare version otherwise, since an uninstalled
	// series has no single tag yet. Both are empty when Status is
	// StatusUnknown.
	Version string `json:"version"`
	Label   string `json:"label"`

	VersionID string `json:"versionId"` // the installed build's ID; "" unless Status is StatusReady
	IsMono    bool   `json:"isMono"`    // whether the target is a .NET build

	// Warning is a non-blocking caution to show alongside the row: the
	// chosen editor is a different series than the project declares (which
	// will upgrade the project files), the project needs .NET and this
	// build has none, or an explicit pin has gone missing. Empty when
	// there's nothing to flag.
	Warning string `json:"warning"`

	// WillUpgrade marks the one warning that isn't merely advisory: this
	// project would open in a newer series than it declares, so Godot will
	// rewrite its files to the new format and there is no way back. Kept as
	// its own flag rather than left for the UI to infer from Warning's
	// wording, because it gates a confirmation prompt.
	WillUpgrade bool `json:"willUpgrade"`
}

// ResolveMode reports which mode a project is in, defaulting records
// written before VersionMode existed: one carrying an explicit pin keeps
// it, and everything else lands on ModeProject. That default is the whole
// point of the mode -- the behaviour it replaces was "launch the newest
// installed build of the same major", which would happily open a 4.2
// project in a 4.8 editor and trigger Godot's one-way project upgrade.
func ResolveMode(p Project) string {
	switch p.VersionMode {
	case ModeProject, ModeDefault, ModePinned:
		return p.VersionMode
	}
	if p.PinnedVersionID != "" {
		return ModePinned
	}
	return ModeProject
}
