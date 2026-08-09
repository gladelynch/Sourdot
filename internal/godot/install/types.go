// Package install handles downloading, extracting, verifying, and placing
// Godot versions on disk, plus admin-free resolution of which installed
// version to launch for a given project (deliberately not symlink-based --
// see the plan's rationale for avoiding Windows admin/Developer-Mode
// friction). Implemented in M1.
package install

import "time"

// InstalledVersion is a persisted record of a Godot version extracted onto
// disk, keyed by a synthetic ID unique across OS/arch/mono variants (e.g.
// "4.2.1-stable-mono-linux-x86_64") so that multiple installs never collide
// in the versions directory -- this is what avoids the "generic Godot.app
// naming collision" bug seen in competitor tools on macOS.
type InstalledVersion struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Major       int       `json:"major"`
	Minor       int       `json:"minor"`
	Patch       int       `json:"patch"`
	Label       string    `json:"label"` // "stable", "rc1", "beta2", ...
	IsMono      bool      `json:"isMono"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	InstallPath string    `json:"installPath"` // directory this version was extracted into
	BinaryPath  string    `json:"binaryPath"`  // path to the actual executable / .app bundle entry point
	SourceURL   string    `json:"sourceURL"`   // release asset URL it came from
	InstalledAt time.Time `json:"installedAt"`
	SizeBytes   int64     `json:"sizeBytes"`
}
