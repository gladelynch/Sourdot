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
# Everything runs in the foreground. Closing the app window, Ctrl-C, or
# closing the terminal stops the lot -- there is no background process to
# track down.
#
# Usage:
#   ./dev.sh              # run it; Ctrl-C to stop
#   ./dev.sh --gui ...    # no terminal: output goes to dev.log, and failures
#                         # pop up a dialog (Sourdot-Dev.desktop passes this)
#
# Any other arguments are passed straight through to `wails dev`.

set -uo pipefail

SELF="$(readlink -f "$0")"
cd "$(dirname "$SELF")" || exit 1

GUI=0
if [[ "${1:-}" == "--gui" ]]; then
	GUI=1
	shift
fi

# `kill 0` in the exit trap below signals this script's whole process group.
# From a shell that group is this script and its children, but a desktop
# launcher may leave it in the launcher's own group, so with --gui start a new
# session first. (setsid doesn't fork when the caller isn't a group leader, so
# this keeps the same pid.) Not done from a terminal: a new session has no
# controlling tty, and Ctrl-C would stop reaching it.
if ((GUI)) && [[ "$(ps -o pgid= -p $$ | tr -d ' ')" != "$$" ]]; then
	exec setsid "$SELF" --gui "$@"
fi

# With no terminal there's nowhere for output to go, so keep it in dev.log,
# next to the database and sourdot.log (mirrors platform.ConfigDir()).
if ((GUI)); then
	LOG="${XDG_CONFIG_HOME:-$HOME/.config}/sourdot/dev.log"
	mkdir -p "$(dirname "$LOG")"
	exec >"$LOG" 2>&1
fi

# fail reports an error and exits. With --gui nobody is watching the output,
# so it also shows a dialog with the end of the log, which is where the
# compiler error will be.
fail() {
	echo >&2
	echo "error: $*" >&2
	if ((GUI)); then
		local msg
		msg="$*"$'\n\n'"$(tail -n 20 "$LOG")"$'\n\n'"Full output: $LOG"
		if command -v zenity >/dev/null 2>&1; then
			zenity --error --no-markup --title "Sourdot (dev)" --width 700 --text "$msg"
		elif command -v kdialog >/dev/null 2>&1; then
			kdialog --title "Sourdot (dev)" --error "$msg"
		elif command -v notify-send >/dev/null 2>&1; then
			notify-send -u critical "Sourdot (dev)" "$*. See $LOG"
		fi
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
	((GUI)) && command -v notify-send >/dev/null 2>&1 &&
		notify-send "Sourdot (dev)" "Installing the wails CLI (one-time); the app will open when it's done."
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
