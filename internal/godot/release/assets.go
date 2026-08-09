package release

import (
	"fmt"
	"regexp"
	"runtime"
)

// ClassifiedAsset is the platform/variant metadata extracted from an
// asset's filename, normalized to one vocabulary regardless of the source
// major version's own naming quirks -- callers never need to know that
// Godot 3.x calls Linux "x11" and macOS "osx" while 4.x calls them "linux"
// and "macos".
type ClassifiedAsset struct {
	OS     string // "linux" | "macos" | "windows"
	Arch   string // "x86_64" | "x86_32" | "arm64" | "arm32" | "universal"
	IsMono bool
}

type assetPattern struct {
	re      *regexp.Regexp
	os      string
	isMono  bool
	arch    string            // fixed arch; used when archMap is nil
	archMap map[string]string // capture-group value -> normalized arch
}

var bitnessArchMap = map[string]string{"32": "x86_32", "64": "x86_64"}

// assetPatterns is a living lookup table, not a perfectly future-proof one
// -- verified Aug 2026 against real godotengine/godot release assets
// spanning 3.5.3 through 4.7.1 (fixtures in assets_test.go are drawn
// directly from those releases). New Godot versions may need a row added
// here if the project ever changes its naming convention again.
// The version/label token between "Godot_v" and the platform suffix (e.g.
// "4.7.1-stable") never itself contains an underscore -- Godot tags are
// digits/dots/hyphens/letters only. So the wildcard matching that token
// deliberately excludes underscore, to stop it from ambiguously swallowing
// a filename's "_mono_" segment (which would otherwise make e.g. the
// standard macOS pattern also match the mono macOS filename, since both
// share the same "_macos.universal.zip" suffix).
const versionToken = `[A-Za-z0-9.+-]+`

var assetPatterns = []assetPattern{
	// Linux, 4.x: "Godot_v4.7.1-stable_linux.x86_64.zip"
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_linux\.(x86_64|x86_32|arm64|arm32)\.zip$`), os: "linux", isMono: false},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_linux_(x86_64|x86_32|arm64|arm32)\.zip$`), os: "linux", isMono: true},

	// Linux, 3.x: named "x11", bitness only (no explicit x86 prefix) --
	// e.g. "Godot_v3.6-stable_x11.64.zip" / "..._mono_x11_64.zip".
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_x11\.(32|64)\.zip$`), os: "linux", isMono: false, archMap: bitnessArchMap},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_x11_(32|64)\.zip$`), os: "linux", isMono: true, archMap: bitnessArchMap},

	// macOS, 4.x: named "macos", always a single universal binary.
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_macos\.universal\.zip$`), os: "macos", isMono: false, arch: "universal"},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_macos\.universal\.zip$`), os: "macos", isMono: true, arch: "universal"},

	// macOS, 3.x: named "osx" instead of "macos".
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_osx\.universal\.zip$`), os: "macos", isMono: false, arch: "universal"},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_osx\.universal\.zip$`), os: "macos", isMono: true, arch: "universal"},

	// Windows, 3.x & 4.x share "win32"/"win64" naming for standard builds
	// (with a ".exe" segment); the mono variant drops ".exe" entirely --
	// e.g. standard "Godot_v4.7.1-stable_win64.exe.zip" vs. mono
	// "Godot_v4.7.1-stable_mono_win64.zip".
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_win(32|64)\.exe\.zip$`), os: "windows", isMono: false, archMap: bitnessArchMap},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_win(32|64)\.zip$`), os: "windows", isMono: true, archMap: bitnessArchMap},

	// Windows arm64, 4.x only.
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_windows_arm64\.exe\.zip$`), os: "windows", isMono: false, arch: "arm64"},
	{re: regexp.MustCompile(`^Godot_v` + versionToken + `_mono_windows_arm64\.zip$`), os: "windows", isMono: true, arch: "arm64"},
}

// Classify extracts platform/variant metadata from an editor download's
// filename, or ok=false if name isn't a recognized editor zip (source
// tarballs, export templates, Android/AAR assets, checksum files, etc. --
// all deliberately out of v1's scope).
func Classify(name string) (ClassifiedAsset, bool) {
	for _, p := range assetPatterns {
		m := p.re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		arch := p.arch
		switch {
		case p.archMap != nil:
			arch = p.archMap[m[1]]
		case len(m) > 1:
			// No archMap but a capture group exists: the captured value
			// is already in normalized form (e.g. 4.x's "x86_64"), so
			// pass it through as-is.
			arch = m[1]
		}
		return ClassifiedAsset{OS: p.os, Arch: arch, IsMono: p.isMono}, true
	}
	return ClassifiedAsset{}, false
}

// HostPlatform returns the normalized (os, arch) this binary is running on,
// in the same vocabulary Classify produces, so callers can filter a
// Release's Assets by direct equality against it.
func HostPlatform() (osName, arch string) {
	if runtime.GOOS == "darwin" {
		return "macos", "universal"
	}
	osName = runtime.GOOS // "linux" / "windows" already match Godot's naming

	switch runtime.GOARCH {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "x86_32"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm32"
	default:
		arch = runtime.GOARCH
	}
	return osName, arch
}

// FindHostAsset returns the Asset in rel matching the current host
// platform and the requested standard/mono variant.
func FindHostAsset(rel Release, isMono bool) (Asset, error) {
	wantOS, wantArch := HostPlatform()
	for _, a := range rel.Assets {
		if a.OS == wantOS && a.Arch == wantArch && a.IsMono == isMono {
			return a, nil
		}
	}
	variant := ""
	if isMono {
		variant = " (mono)"
	}
	return Asset{}, fmt.Errorf("no %s/%s%s asset found in release %s", wantOS, wantArch, variant, rel.TagName)
}
