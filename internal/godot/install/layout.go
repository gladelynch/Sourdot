package install

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// SyntheticID builds the collision-proof, unique identifier used both as
// InstalledVersion.ID and the versions-directory subfolder name -- every
// install gets its own uniquely-named directory regardless of what Godot
// names the binary/bundle inside it, which is what avoids the "generic
// Godot.app naming collision" bug seen in competitor tools on macOS.
func SyntheticID(tagName, osName, arch string, isMono bool) string {
	variant := osName
	if isMono {
		variant = "mono-" + osName
	}
	return fmt.Sprintf("%s-%s-%s", tagName, variant, arch)
}

// InstallDir returns the directory a version with the given synthetic ID
// is (or will be) extracted into, under versionsDir.
func InstallDir(versionsDir, id string) string {
	return filepath.Join(versionsDir, id)
}

// ResolveBinaryPath finds the actual Godot executable inside a freshly
// extracted install directory.
//
// This deliberately doesn't try to predict the exact filename from the
// asset name -- Godot's own naming is inconsistent between a mono
// install's wrapping folder and the binary inside it (verified Aug 2026:
// folder "Godot_v4.7.1-stable_mono_linux_x86_64" contains a binary named
// "Godot_v4.7.1-stable_mono_linux.x86_64" -- underscore in the folder,
// dot in the binary). Instead it searches the way a human would: on
// Windows, the first non-console .exe; on macOS, the first .app bundle;
// elsewhere, the first executable regular file -- skipping the
// GodotSharp/ support folder mono installs ship alongside the binary.
func ResolveBinaryPath(installDir, osName string) (string, error) {
	var found string
	walkErr := filepath.WalkDir(installDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}

		// Match by basename rather than a path prefix: mono installs add
		// an extra wrapping folder around GodotSharp/ (see doc comment),
		// so its depth isn't fixed. A path-prefix check anchored at
		// installDir would miss it and let the walk descend into
		// GodotSharp/Tools/*.dll, some of which carry a spurious
		// executable bit in the zip's stored permissions and would
		// otherwise be misidentified as the real binary below.
		if path != installDir && d.IsDir() && d.Name() == "GodotSharp" {
			return filepath.SkipDir
		}

		switch osName {
		case "windows":
			lower := strings.ToLower(d.Name())
			if !d.IsDir() && strings.HasSuffix(lower, ".exe") && !strings.HasSuffix(lower, "_console.exe") {
				found = path
			}
		case "macos":
			if d.IsDir() && strings.HasSuffix(d.Name(), ".app") {
				found = path
				return filepath.SkipDir
			}
		default: // linux and anything else: first executable regular file
			if !d.IsDir() {
				if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
					found = path
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if found == "" {
		return "", fmt.Errorf("no Godot binary found under %s", installDir)
	}
	return found, nil
}

// DirSize returns the total size in bytes of all regular files under root.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}
