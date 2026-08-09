// Package project scans folders for Godot projects, parses project.godot
// for engine-version/renderer/C# signals, and resolves per-project version
// pins (own format plus read-compat for incumbent formats). Implemented in
// M3.
package project

import "time"

// Project is a tracked Godot project folder, persisted in
// store.BucketProjects.
type Project struct {
	ID               string // hash of the resolved absolute path
	Path             string // absolute path to the project root (contains project.godot)
	Name             string // from [application] config/name, falling back to the dir name
	DetectedVersion  string // parsed from project.godot's config_version
	DetectedRenderer string // "forward_plus" | "mobile" | "compatibility" (4.x only)
	UsesCSharp       bool   // [dotnet] section present, or a sibling .csproj
	PinnedVersionID  string // references an install.InstalledVersion.ID; empty = use DetectedVersion
	Favorite         bool
	Tags             []string
	LastOpenedAt     time.Time
	AddedAt          time.Time
	Missing          bool // path no longer exists on disk; surfaced, not silently dropped
}
