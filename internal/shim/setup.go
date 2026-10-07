package shim

import (
	"path/filepath"
	"strings"
)

// Setup is what the Settings view shows for putting the launcher on PATH.
// Sourdot never edits a shell profile itself: those files are the user's,
// and a line appended behind their back is one they'll find later and not
// know where it came from. So it hands over the lines to paste instead.
type Setup struct {
	Profile string   `json:"profile"` // where the lines go, e.g. "~/.bashrc"; "" on Windows, where they're run once rather than saved
	Lines   []string `json:"lines"`
}

// SetupFor builds the PATH and GODOT_BIN lines for goos and the user's
// login shell (the value of $SHELL; ignored on Windows). GODOT_BIN is the
// variable GdUnit4's test runner reads for the engine binary.
func SetupFor(goos, shell, binDir, home string) Setup {
	launcher := filepath.Join(binDir, FileName(goos))

	if goos == "windows" {
		return Setup{Lines: []string{
			`[Environment]::SetEnvironmentVariable("Path", ` + psQuote(binDir) + ` + ";" + [Environment]::GetEnvironmentVariable("Path", "User"), "User")`,
			`[Environment]::SetEnvironmentVariable("GODOT_BIN", ` + psQuote(launcher) + `, "User")`,
		}}
	}

	switch filepath.Base(shell) {
	case "fish":
		return Setup{Profile: "~/.config/fish/config.fish", Lines: []string{
			"fish_add_path " + homeQuote(binDir, home),
			"set -gx GODOT_BIN " + homeQuote(launcher, home),
		}}
	case "zsh":
		return Setup{Profile: "~/.zshrc", Lines: posixLines(binDir, launcher, home)}
	default:
		return Setup{Profile: "~/.bashrc", Lines: posixLines(binDir, launcher, home)}
	}
}

func posixLines(binDir, launcher, home string) []string {
	// Prepended rather than appended: a distro-packaged `godot` already on
	// PATH would otherwise win, and it knows nothing about pins.
	dir := homeQuote(binDir, home)
	return []string{
		`export PATH=` + dir + `:"$PATH"`,
		`export GODOT_BIN=` + homeQuote(launcher, home),
	}
}

// homeQuote quotes path for a POSIX shell or fish, writing it relative to
// $HOME when it's under it. That keeps the pasted line readable, and
// correct in a dotfiles repo shared between machines with different
// usernames.
func homeQuote(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+"/") && !strings.ContainsAny(path, "\"$`\\") {
		return `"$HOME/` + strings.TrimPrefix(path, home+"/") + `"`
	}
	return shQuote(path)
}

// psQuote single-quotes s for PowerShell, where a doubled ' is the only
// escape.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
