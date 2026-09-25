#!/usr/bin/env bash
#
# Dev launcher for Sourdot.
#
# Runs `wails dev`, which rebuilds + restarts the Go backend when any .go file
# changes and live-reloads the frontend when anything under frontend/dist
# changes. frontend/dist is generated, so this also runs the frontend bundler
# (tools/frontendbuild, an esbuild-in-Go wrapper) in watch mode alongside it:
# edit frontend/src or frontend/public, esbuild rewrites dist, wails reloads.
#
# Everything runs in the foreground of this terminal. Ctrl-C or closing the
# window stops the lot -- there is no background process to track down.
#
# Usage:
#   ./dev.sh              # run it; Ctrl-C to stop
#   ./dev.sh --hold ...   # on failure, wait for Enter before exiting, so the
#                         # error stays readable (Sourdot-Dev.desktop passes this)
#
# Any other arguments are passed straight through to `wails dev`.

set -uo pipefail

cd "$(dirname "$(readlink -f "$0")")" || exit 1

HOLD=0
if [[ "${1:-}" == "--hold" ]]; then
	HOLD=1
	shift
fi

# fail reports an error and exits. Launched from Sourdot-Dev.desktop, the terminal
# window closes the moment this script does, so --hold keeps it up until the
# error has been read.
fail() {
	echo >&2
	echo "error: $*" >&2
	if ((HOLD)); then
		echo >&2
		read -rp "Press Enter to close." _
	fi
	exit 1
}

# --- Toolchain ----------------------------------------------------------------
# Launched from a desktop entry, PATH is the session's, not your shell's, so
# anything added in ~/.bashrc is missing. Add the usual Go install locations
# directly rather than depending on how this particular machine was set up.
for dir in /usr/local/go/bin "$HOME/go/bin" "$HOME/.local/go/bin" "$HOME/sdk/go/bin"; do
	[[ -d "$dir" ]] && case ":$PATH:" in *":$dir:"*) ;; *) PATH="$dir:$PATH" ;; esac
done
export PATH

command -v go >/dev/null 2>&1 ||
	fail "'go' not found. Install Go (https://go.dev/dl/), then run this again."

# The wails CLI installs to GOBIN (or GOPATH/bin), which may be somewhere
# other than the defaults above.
GOBIN="$(go env GOBIN)"
[[ -n "$GOBIN" ]] || GOBIN="$(go env GOPATH)/bin"
PATH="$GOBIN:$PATH"

# Install the wails CLI on first run, at the version go.mod pins, so a fresh
# machine needs nothing beyond Go (and the system libraries wails links to).
if ! command -v wails >/dev/null 2>&1; then
	WAILS_VERSION="$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)" ||
		fail "couldn't read the wails version from go.mod."
	echo ">>> wails not found; installing $WAILS_VERSION (one-time)"
	go install "github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION" ||
		fail "installing wails failed; see the output above."
fi

# Linux distros that ship only webkit2gtk-4.1 need this build tag; see the
# Makefile and the README's "Building" section. Windows/macOS don't.
TAGS=()
if [[ "$(uname -s)" == "Linux" ]]; then
	TAGS=(-tags webkit2_41)
fi

# --- Run ----------------------------------------------------------------------

# Build frontend/dist once up front. The watcher would do this itself a moment
# later, but doing it here means wails dev never starts against a half-written
# dist, and a syntax error is reported before the window opens.
go run ./tools/frontendbuild ||
	fail "frontend build failed; fix the errors above and run this again."

# When this script exits, for any reason, take the frontend watcher and
# everything wails started down with it. `kill 0` signals this script's whole
# process group; the trap is cleared first so it doesn't run twice.
trap 'trap - EXIT INT TERM HUP; kill 0 2>/dev/null' EXIT INT TERM HUP

go run ./tools/frontendbuild -watch &

# -assetdir points the dev server at the on-disk frontend instead of the
# embedded copy, which is what makes frontend edits hot-reload.
echo ">>> wails dev ${TAGS[*]} -assetdir frontend/dist $*"
echo ">>> edit Go files to rebuild, frontend/src to re-bundle; Ctrl-C to stop"
echo
wails dev "${TAGS[@]}" -assetdir frontend/dist "$@" ||
	fail "wails dev exited with an error; see the output above."
