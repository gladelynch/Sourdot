package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProjectInfo is the handful of signals Sourdot needs out of a project.godot
// file: its declared name, the config_version integer (a reliable signal
// for the major engine family -- 4 means Godot 3.x, 5 means Godot 4.x --
// unlike parsing the freeform config/features version string), whether
// it's a C# project, its config/icon path (if any), and a best-effort
// precise engine version parsed from config/features.
type ProjectInfo struct {
	Name           string
	ConfigVersion  int
	UsesCSharp     bool
	Icon           string // config/icon value, e.g. "res://icon.svg"; empty if unset
	FeatureVersion string // e.g. "4.3", parsed from config/features; "" if not found/parseable -- display only, see MajorFromConfigVersion for the resolution-authoritative major
}

// ParseProjectGodot does a minimal hand-rolled scan of a project.godot
// file -- not a generic INI parser, since only a few keys matter here and
// Godot's format has quirks (PackedStringArray(...) values, uid://
// references) that a generic parser would need real customization to
// handle anyway.
func ParseProjectGodot(path string) (ProjectInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return ProjectInfo{}, err
	}
	defer f.Close()

	var info ProjectInfo
	inApplication := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			inApplication = section == "application"
			if section == "dotnet" {
				info.UsesCSharp = true
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch {
		case key == "config_version":
			if n, err := strconv.Atoi(value); err == nil {
				info.ConfigVersion = n
			}
		case inApplication && key == "config/name":
			info.Name = unquote(value)
		case inApplication && key == "config/icon":
			info.Icon = unquote(value)
		case inApplication && key == "config/features":
			info.FeatureVersion = parseFeatureVersion(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return ProjectInfo{}, err
	}

	if !info.UsesCSharp && hasCSProjSibling(filepath.Dir(path)) {
		info.UsesCSharp = true
	}

	return info, nil
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseFeatureVersion pulls the engine-version token out of a
// config/features value, e.g. `PackedStringArray("4.3", "Forward Plus")` ->
// "4.3". Returns "" if no version-shaped token is found. Best-effort only:
// Godot's feature tags track the minor release ("4.3"), not the exact
// patch/beta build, so this can't distinguish "4.8" stable from a "4.8"
// beta -- see ProjectInfo.FeatureVersion.
func parseFeatureVersion(value string) string {
	inner := value
	if i := strings.Index(inner, "("); i >= 0 {
		inner = inner[i+1:]
	}
	inner = strings.TrimSuffix(strings.TrimSpace(inner), ")")
	for _, tok := range strings.Split(inner, ",") {
		tok = unquote(strings.TrimSpace(tok))
		if isVersionToken(tok) {
			return tok
		}
	}
	return ""
}

// isVersionToken reports whether s looks like a version number ("4.3"):
// digits and dots only, with at least one dot -- enough to distinguish it
// from feature tags like "Forward Plus" or "C#" in the same array.
func isVersionToken(s string) bool {
	if s == "" {
		return false
	}
	hasDot := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			hasDot = true
		default:
			return false
		}
	}
	return hasDot
}

func hasCSProjSibling(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".csproj") {
			return true
		}
	}
	return false
}

// MajorFromConfigVersion maps project.godot's config_version to the Godot
// major version family it belongs to. Returns "" if cv isn't a recognized
// value.
func MajorFromConfigVersion(cv int) string {
	switch cv {
	case 5:
		return "4"
	case 4:
		return "3"
	default:
		return ""
	}
}
