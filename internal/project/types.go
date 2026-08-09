// Package project scans folders for Godot projects, parses project.godot
// for engine-version/C# signals, and resolves per-project version pins
// (own format plus read-compat for incumbent formats).
package project

import "time"

// Project is a tracked Godot project folder, persisted in
// store.BucketProjects.
type Project struct {
	ID               string    `json:"id"`               // hash of the resolved absolute path
	Path             string    `json:"path"`             // absolute path to the project root (contains project.godot)
	Name             string    `json:"name"`             // from [application] config/name, falling back to the dir name
	DetectedVersion  string    `json:"detectedVersion"`  // major only ("3" or "4"), from project.godot's config_version -- see ParseProjectGodot; resolution-authoritative, see ProjectManager.ResolveVersionSpec
	DetectedRenderer string    `json:"detectedRenderer"` // best-effort; may be empty
	UsesCSharp       bool      `json:"usesCSharp"`       // [dotnet] section present, or a sibling .csproj
	PinnedVersionID  string    `json:"pinnedVersionId"`  // references an install.InstalledVersion.ID; empty = fall through the pin precedence chain
	Favorite         bool      `json:"favorite"`
	Tags             []string  `json:"tags"`
	LastOpenedAt     time.Time `json:"lastOpenedAt"`
	AddedAt          time.Time `json:"addedAt"`
	Missing          bool      `json:"missing"` // path no longer exists on disk; surfaced, not silently dropped

	// DetectedVersionLabel and Thumbnail are display-only and recomputed
	// live from disk on every ProjectManager.ListProjects call (like
	// Missing above) rather than persisted -- so an icon swap or engine
	// upgrade shows up immediately without a remove/re-add. Zero-valued on
	// a Project fetched any other way (e.g. GetProject).
	DetectedVersionLabel string `json:"detectedVersionLabel"` // best-effort precise version ("4.3"), from config/features; falls back to DetectedVersion when not parseable -- see ParseProjectGodot
	Thumbnail            string `json:"thumbnail"`            // data: URI of the project's icon, or "" if none found -- see ResolveThumbnail
}
