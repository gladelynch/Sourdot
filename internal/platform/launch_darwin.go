//go:build darwin

package platform

import "os/exec"

// Launch starts the Godot editor for projectPath. On macOS,
// install.ResolveBinaryPath returns the .app bundle directory rather than
// an inner Mach-O executable, so this goes through `open`, which launches
// bundles correctly and forwards --args to the wrapped process. -n starts
// a new instance even if a bundle with the same *internal* name ("Godot")
// is already running from a different install directory -- a real
// scenario here, since every version unpacks into its own uniquely-named
// parent directory but each .app bundle inside keeps Apple's expected
// name.
//
// NOTE: written to documented `open`/Godot CLI behavior but not yet
// verified on real macOS hardware -- flagged in the plan's open risks for
// M6's cross-platform pass.
func Launch(bundlePath, projectPath string) (*exec.Cmd, error) {
	cmd := exec.Command("open", "-n", bundlePath, "--args", "--path", projectPath, "--editor")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
