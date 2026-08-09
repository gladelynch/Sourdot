// Package release discovers available Godot versions via the GitHub
// Releases API and matches release assets to the current OS/arch/mono
// variant. M1 implements the client and asset-pattern matcher; this file
// just establishes the shapes everything else builds on.
package release

import "time"

// Release is a single Godot release as reported by the GitHub Releases API.
//
// Open question (M1 spike, see plan): whether the source repo is
// godotengine/godot or godotengine/godot-builds for recent versions. Both
// expose the same release/asset JSON shape, so this type is source-agnostic
// -- only client.go's repo constant changes depending on the answer.
type Release struct {
	TagName     string // e.g. "4.2.1-stable"
	Major       int
	Minor       int
	Patch       int
	Label       string // "stable", "rc1", "beta2", ...
	PublishedAt time.Time
	BodyMD      string // release notes body, rendered for changelog preview in M2
	Assets      []Asset
}

// Asset is a single downloadable file attached to a Release.
type Asset struct {
	Name        string
	DownloadURL string
	SizeBytes   int64
	OS          string // "windows" | "linux" | "macos"
	Arch        string // "x86_64" | "arm64" | "x86_32"
	IsMono      bool
}
