package project

import (
	"os"
	"path/filepath"
	"strings"
)

// PinSpec is a resolved version target: a plain version string (e.g.
// "4.2.1", matching install.InstalledVersion.Version) and whether the
// mono/.NET build is wanted.
type PinSpec struct {
	Version string
	IsMono  bool
}

// ParsePinValue parses a single pin value like "4.2.1" or "4.2.1-mono".
// Deliberately excludes Godot's own release labels (no "-stable"/"-rc1"):
// v1 only targets the stable channel, so a pin only ever needs to name a
// version and a standard/mono choice.
func ParsePinValue(s string) PinSpec {
	s = strings.TrimSpace(s)
	if v, ok := strings.CutSuffix(s, "-mono"); ok {
		return PinSpec{Version: v, IsMono: true}
	}
	return PinSpec{Version: s}
}

// ReadPinFile reads Sourdot's own .sourdot-version pin file from dir: a
// plain single-line file with no OS/arch baked in, so a pin checked into
// git stays portable across machines.
func ReadPinFile(dir string) (PinSpec, bool) {
	return readSingleLinePin(filepath.Join(dir, ".sourdot-version"))
}

// ReadGodotVersionFile reads the incumbent .godot-version convention
// (same plain-text format), read-only, for easy migration from tools that
// use it.
func ReadGodotVersionFile(dir string) (PinSpec, bool) {
	return readSingleLinePin(filepath.Join(dir, ".godot-version"))
}

// ReadToolVersionsFile reads a "godot <version>" line out of an
// asdf/mise-style .tool-versions file, read-only, ignoring every other
// tool line.
func ReadToolVersionsFile(dir string) (PinSpec, bool) {
	data, err := os.ReadFile(filepath.Join(dir, ".tool-versions"))
	if err != nil {
		return PinSpec{}, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "godot" {
			return ParsePinValue(fields[1]), true
		}
	}
	return PinSpec{}, false
}

func readSingleLinePin(path string) (PinSpec, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PinSpec{}, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return ParsePinValue(line), true
		}
	}
	return PinSpec{}, false
}
