// Package project scans folders for Godot projects, parses project.godot
// for engine-version/C# signals, and resolves per-project version pins
// (own format plus read-compat for incumbent formats).
package project

import "time"

// Project is a tracked Godot project folder, persisted in
// store.BucketProjects.
type Project struct {
	ID               string    `json:"id"`   // hash of the resolved absolute path
	Path             string    `json:"path"` // absolute path to the project root (contains project.godot)
	Name             string    `json:"name"` // from [application] config/name, falling back to the dir name
	DetectedVersion  string    `json:"detectedVersion"`  // major only ("3" or "4"), from project.godot's config_version -- see ParseProjectGodot
	DetectedRenderer string    `json:"detectedRenderer"` // best-effort; may be empty
	UsesCSharp       bool      `json:"usesCSharp"`       // [dotnet] section present, or a sibling .csproj
	PinnedVersionID  string    `json:"pinnedVersionId"`  // references an install.InstalledVersion.ID; empty = fall through the pin precedence chain
	Favorite         bool      `json:"favorite"`
	Tags             []string  `json:"tags"`
	LastOpenedAt     time.Time `json:"lastOpenedAt"`
	AddedAt          time.Time `json:"addedAt"`
	Missing          bool      `json:"missing"` // path no longer exists on disk; surfaced, not silently dropped
}
