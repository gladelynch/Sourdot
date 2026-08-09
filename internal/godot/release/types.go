// Package release discovers available Godot versions via the GitHub
// Releases API and matches release assets to the current OS/arch/mono
// variant.
package release

import "time"

// Release is a single Godot release as reported by the GitHub Releases API.
//
// Verified Aug 2026 against godotengine/godot: godotengine/godot-builds
// (which hosts rc/beta/dev pre-releases, out of v1's stable-only scope)
// uses an identical asset-naming scheme, so this type and the Classify
// table both work unchanged if pre-release channels are added later --
// only the client's source-repo constant would need to change.
type Release struct {
	TagName     string    `json:"tagName"` // e.g. "4.2.1-stable"
	Major       int       `json:"major"`
	Minor       int       `json:"minor"`
	Patch       int       `json:"patch"`
	Label       string    `json:"label"` // "stable", "rc1", "beta2", ...
	PublishedAt time.Time `json:"publishedAt"`
	BodyMD      string    `json:"bodyMD"` // release notes body, rendered for changelog preview in M2

	// ChecksumsURL points at the release's SHA512-SUMS.txt asset, if
	// published. Verified present for every stable release checked from
	// 3.5.3 through 4.7.1, but GitHub's release format doesn't guarantee
	// it, so this can be empty -- install.VerifyChecksum treats that as
	// "skip verification", not a hard failure.
	ChecksumsURL string `json:"checksumsURL"`

	// Assets is filtered to only the classified editor download zips (see
	// Classify) -- source tarballs, export templates, Android/AAR
	// artifacts, and checksum files are deliberately excluded here since
	// none of them are installable editor versions.
	Assets []Asset `json:"assets"`
}

// Asset is a single downloadable editor zip attached to a Release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"downloadURL"`
	SizeBytes   int64  `json:"sizeBytes"`
	OS          string `json:"os"`   // "linux" | "macos" | "windows"
	Arch        string `json:"arch"` // "x86_64" | "x86_32" | "arm64" | "arm32" | "universal"
	IsMono      bool   `json:"isMono"`
}
