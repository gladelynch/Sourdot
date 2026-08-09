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
