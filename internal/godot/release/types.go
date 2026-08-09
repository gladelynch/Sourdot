// Package release discovers available Godot versions via the GitHub
// Releases API and matches release assets to the current OS/arch/mono
// variant.
package release

import "time"

// Release is a single Godot release as reported by the GitHub Releases API.
//
// Sourced from godotengine/godot-builds, which is a strict superset of
// godotengine/godot: it carries every stable tag plus all the dev/alpha/
// beta/rc pre-releases, using an identical asset-naming scheme (so the
// Classify table below works unchanged across both).
type Release struct {
	TagName     string    `json:"tagName"` // e.g. "4.2.1-stable"
	Series      string    `json:"series"`  // the tag's version part, e.g. "4.2.1" -- the grouping key
	Major       int       `json:"major"`
	Minor       int       `json:"minor"`
	Patch       int       `json:"patch"`
	Label       string    `json:"label"`   // "stable", "rc1", "beta2", ...
	Channel     string    `json:"channel"` // Label with its sequence number stripped: "stable", "rc", "beta", "alpha", "dev"
	PublishedAt time.Time `json:"publishedAt"`
	BodyMD      string    `json:"bodyMD"` // raw release-notes body; stripped from the catalog sent to the UI to keep the payload small

	// ReleaseNotesURL and ChangelogURL are the human-facing write-ups for
	// this release, opened in the user's real browser (never in-app). Both
	// are taken from the release body when it carries them and otherwise
	// reconstructed; see notes.go. ReleaseNotesURL can be empty, the
	// changelog one never is.
	ReleaseNotesURL string `json:"releaseNotesURL"`
	ChangelogURL    string `json:"changelogURL"`

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
