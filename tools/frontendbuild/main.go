// Command frontendbuild bundles frontend/src into frontend/dist.
//
// It exists so the frontend can use ES modules, JSX and Preact without the
// project ever growing a Node toolchain: esbuild ships a Go API, so the
// bundler is just another Go dependency (it pulls in golang.org/x/sys,
// which the Wails dependency tree already carries) and the whole build runs
// under `go run`.
//
//	go run ./tools/frontendbuild            # one-shot build
//	go run ./tools/frontendbuild -watch     # rebuild on every save
//
// Layout:
//
//	frontend/vendor/preact  vendored Preact ESM, resolved via Alias below
//	frontend/src            JS/JSX sources; src/main.jsx is the entry point
//	frontend/public         index.html, css/, assets/ -- copied verbatim
//	frontend/dist           generated output; gitignored, embedded by main.go
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

const (
	srcDir    = "frontend/src"
	publicDir = "frontend/public"
	vendorDir = "frontend/vendor"
	outDir    = "frontend/dist"
	entry     = srcDir + "/main.jsx"
)

func main() {
	watch := flag.Bool("watch", false, "rebuild whenever a source file changes")
	minify := flag.Bool("minify", false, "minify the bundle (production builds)")
	flag.Parse()

	// Run from the repo root regardless of where the caller invoked us, so
	// the relative paths above always mean the same thing.
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	if err := os.Chdir(root); err != nil {
		fail(err)
	}

	// public/ is copied before the first bundle so a watch session serves a
	// complete dist from the moment it starts.
	if err := copyTree(publicDir, outDir); err != nil {
		fail(fmt.Errorf("copying %s: %w", publicDir, err))
	}

	ctx, cerr := api.Context(buildOptions(*minify))
	if cerr != nil {
		fail(fmt.Errorf("esbuild: %v", cerr))
	}
	defer ctx.Dispose()

	if !*watch {
		if report(ctx.Rebuild()) {
			os.Exit(1)
		}
		fmt.Printf("frontendbuild: wrote %s/js/app.js\n", outDir)
		return
	}

	// esbuild watches everything reachable from the entry point itself. It
	// has no idea about public/, which isn't part of the module graph, so
	// that gets its own poll -- cheap at this size, and it keeps index.html
	// and the CSS hot-reloading the way they did before the build step.
	report(ctx.Rebuild())
	if werr := ctx.Watch(api.WatchOptions{}); werr != nil {
		fail(fmt.Errorf("esbuild watch: %v", werr))
	}
	fmt.Printf("frontendbuild: watching %s and %s\n", srcDir, publicDir)
	go watchPublic()

	// Wait for a signal rather than returning: esbuild's watcher runs on its
	// own goroutines and Dispose (deferred above) stops them.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func buildOptions(minify bool) api.BuildOptions {
	return api.BuildOptions{
		EntryPoints: []string{entry},
		Outfile:     outDir + "/js/app.js",
		Bundle:      true,
		Write:       true,
		Format:      api.FormatIIFE,
		// WebKitGTK on Linux is the oldest engine Sourdot ships against, so
		// the bundle targets a baseline all three platforms clear rather
		// than esbuild's default of "whatever the syntax happens to be".
		Target:            api.ES2020,
		Sourcemap:         sourcemapMode(minify),
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		JSX:               api.JSXAutomatic,
		JSXImportSource:   "preact",
		LogLevel:          api.LogLevelSilent, // report() prints these itself
		// Preact is vendored as plain ESM rather than installed, so these
		// three specifiers are wired to the files by hand. This is also what
		// lets the vendored hooks/jsx-runtime modules resolve their own
		// `from "preact"` imports back to the same copy -- without it they'd
		// each pull in a second Preact and the two would not share state.
		//
		// The paths have to be absolute: esbuild resolves a relative alias
		// target against the *importing* file, not the working directory, so
		// a relative one only works for imports that happen to sit at the
		// right depth.
		Alias: map[string]string{
			"preact":             abs(vendorDir + "/preact/preact.module.js"),
			"preact/hooks":       abs(vendorDir + "/preact/hooks.module.js"),
			"preact/jsx-runtime": abs(vendorDir + "/preact/jsxRuntime.module.js"),
		},
	}
}

// abs resolves a repo-relative path for esbuild's alias map, which needs
// absolute targets (see buildOptions).
func abs(path string) string {
	p, err := filepath.Abs(path)
	if err != nil {
		fail(err)
	}
	return p
}

func sourcemapMode(minify bool) api.SourceMap {
	if minify {
		return api.SourceMapNone
	}
	// Inline, because dist/ is served straight off disk by `wails dev` and a
	// linked .map is one more file to keep in sync for no benefit at this size.
	return api.SourceMapInline
}

// report prints esbuild's diagnostics and says whether the build failed.
func report(result api.BuildResult) (failed bool) {
	for _, msg := range append(result.Warnings, result.Errors...) {
		where := ""
		if msg.Location != nil {
			where = fmt.Sprintf("%s:%d:%d: ", msg.Location.File, msg.Location.Line, msg.Location.Column)
		}
		fmt.Fprintf(os.Stderr, "frontendbuild: %s%s\n", where, msg.Text)
	}
	return len(result.Errors) > 0
}

// watchPublic re-copies public/ when anything under it changes. Polling
// mtimes rather than using inotify keeps this dependency-free, and public/
// holds a handful of files that change only when a human edits them.
func watchPublic() {
	last := treeStamp(publicDir)
	for range time.Tick(400 * time.Millisecond) {
		if stamp := treeStamp(publicDir); stamp != last {
			last = stamp
			if err := copyTree(publicDir, outDir); err != nil {
				fmt.Fprintf(os.Stderr, "frontendbuild: %v\n", err)
				continue
			}
			fmt.Printf("frontendbuild: copied %s\n", publicDir)
		}
	}
}

// treeStamp summarises a directory as a string that changes whenever any
// file in it is added, removed, resized or touched.
func treeStamp(dir string) string {
	var sb []byte
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		sb = append(sb, fmt.Sprintf("%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())...)
		return nil
	})
	return string(sb)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// repoRoot walks up from this file's package directory to the directory
// holding go.mod, so `go run ./tools/frontendbuild` works from anywhere.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "frontendbuild: %v\n", err)
	os.Exit(1)
}
