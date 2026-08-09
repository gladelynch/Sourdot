package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProjectInfo is the handful of signals GodotVM needs out of a project.godot
// file: its declared name, the config_version integer (a reliable signal
// for the major engine family -- 4 means Godot 3.x, 5 means Godot 4.x --
// unlike parsing the freeform config/features version string), and whether
// it's a C# project.
type ProjectInfo struct {
	Name          string
	ConfigVersion int
	UsesCSharp    bool
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
