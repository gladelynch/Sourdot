package install

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractZip extracts every entry in zipPath into destDir, which must
// already exist. Rejects any entry whose cleaned path would escape
// destDir (zip-slip protection), preserves the executable bit from the
// zip's stored file mode (Godot's Linux/macOS binaries need it set), and
// recreates symlinks as real symlinks rather than flattening them into
// regular files -- macOS .app bundles routinely contain framework
// symlinks that would otherwise be silently corrupted.
//
// This symlink handling is unrelated to (and doesn't compromise) the
// no-admin-required install/launch design: it only ever runs against the
// asset already matched to the current host OS, and Windows editor zips
// verified so far (win32/win64/mono, standard and .exe variants) contain
// no symlink entries at all -- Windows software isn't packaged that way.
// On the Windows build, os.Symlink requires Developer Mode or admin only
// if it's ever actually called; since that never happens in practice
// here, no elevation prompt is possible. If some future Windows asset
// ever did include one, this would surface as a plain extraction error,
// never a UAC prompt -- GodotVM never requests elevation anywhere.
func ExtractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	cleanDest := filepath.Clean(destDir)
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("refusing to extract %q: escapes destination", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(f, target); err != nil {
			return fmt.Errorf("extracting %q: %w", f.Name, err)
		}
	}
	return nil
}

func extractFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if f.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		_ = os.Remove(target) // symlink() fails if target already exists
		return os.Symlink(string(linkTarget), target)
	}

	mode := f.Mode()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
