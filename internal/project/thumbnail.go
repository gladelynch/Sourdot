package project

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

// maxThumbnailBytes caps how large an icon file Sourdot will inline as a
// data URI -- generous for any reasonable project icon, small enough that
// a config/icon pointed at a huge image doesn't bloat the project list
// response.
const maxThumbnailBytes = 2 << 20 // 2 MiB

// defaultIconCandidates are checked, in order, when project.godot doesn't
// name a config/icon (or the file it names is missing). Godot 4 scaffolds
// icon.svg, Godot 3 scaffolds icon.png; the rest cover common hand-set
// custom icons.
var defaultIconCandidates = []string{"icon.svg", "icon.png", "icon.webp", "icon.jpg", "icon.jpeg"}

// mimeByExt maps the icon extensions Sourdot knows how to inline as a data
// URI. Anything else (e.g. a .ico) is skipped rather than guessed at.
var mimeByExt = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// ResolveThumbnail finds a project's icon on disk and returns it as a
// data: URI ready to drop straight into an <img src>. dir is the project
// root; icon is project.godot's config/icon value (a res:// path, or
// empty). Returns "" if no icon file is found, or it's unreadable, empty,
// too large, or an unrecognized format -- a missing thumbnail is never
// treated as an error, just flavor Sourdot couldn't add.
func ResolveThumbnail(dir, icon string) string {
	path := resolveIconPath(dir, icon)
	if path == "" {
		return ""
	}
	mime, ok := mimeByExt[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxThumbnailBytes {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// resolveIconPath resolves icon (a res:// path from config/icon) against
// dir, falling back to defaultIconCandidates if icon is empty or doesn't
// point at a real file. Rejects anything that would resolve outside dir --
// config/icon is user data, and it shouldn't be able to make Sourdot read
// (and hand back to the UI as a data URI) an arbitrary file elsewhere on
// disk via a "../../" path.
func resolveIconPath(dir, icon string) string {
	if icon != "" {
		rel := strings.TrimPrefix(icon, "res://")
		candidate := filepath.Join(dir, filepath.FromSlash(rel))
		if withinDir(dir, candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	for _, name := range defaultIconCandidates {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func withinDir(dir, candidate string) bool {
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
