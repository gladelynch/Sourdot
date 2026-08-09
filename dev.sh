#!/usr/bin/env bash
#
# Double-clickable dev launcher for Sourdot.
#
# Runs `wails dev`, which rebuilds + restarts the Go backend when any .go file
# changes and live-reloads the frontend when anything under frontend/dist
# changes. Ctrl-C in the terminal window stops it.
#
# Usage:
#   ./dev.sh              # run it (from a terminal, or by double-clicking)
#   ./dev.sh --build      # one-off production build, then launch the binary
#
# Any other arguments are passed straight through to `wails dev`.

set -uo pipefail

cd "$(dirname "$(readlink -f "$0")")" || exit 1

# --- Re-launch in a terminal window if we were double-clicked -----------------
# File managers run scripts with no attached tty, so without this the app's
# logs (and any build errors) would go nowhere.
if [[ ! -t 1 && -z "${SOURDOT_DEV_IN_TERM:-}" ]]; then
	export SOURDOT_DEV_IN_TERM=1
	self="$(readlink -f "$0")"
	for term in konsole ghostty kitty alacritty wezterm foot gnome-terminal \
		xfce4-terminal x-terminal-emulator xterm; do
		command -v "$term" >/dev/null 2>&1 || continue
		case "$term" in
		gnome-terminal) exec "$term" -- "$self" "$@" ;;
		*) exec "$term" -e "$self" "$@" ;;
		esac
	done
	# No terminal emulator found; fall through and run headless anyway.
fi

# Keep the window open on exit so errors are readable when double-clicked.
pause_on_exit() {
	local code=$?
	if [[ -n "${SOURDOT_DEV_IN_TERM:-}" ]]; then
		echo
		echo "--- exited with status $code. Press Enter to close. ---"
		read -r
	fi
	exit "$code"
}
trap pause_on_exit EXIT

# --- Dependency checks --------------------------------------------------------
if ! command -v go >/dev/null 2>&1; then
	echo "error: 'go' not found on PATH. Install Go, then re-run." >&2
	exit 1
fi

# The wails CLI installs to GOPATH/bin, which isn't always on PATH.
GOBIN="$(go env GOBIN)"
[[ -n "$GOBIN" ]] || GOBIN="$(go env GOPATH)/bin"
case ":$PATH:" in
*":$GOBIN:"*) ;;
*) PATH="$GOBIN:$PATH" ;;
esac
export PATH

if ! command -v wails >/dev/null 2>&1; then
	echo "error: 'wails' not found on PATH (looked in $GOBIN)." >&2
	echo "Install it with:" >&2
	echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
	exit 1
fi

# Linux distros that ship only webkit2gtk-4.1 need this build tag; see the
# Makefile and the README's "Building" section. Windows/macOS don't.
TAGS=()
if [[ "$(uname -s)" == "Linux" ]]; then
	TAGS=(-tags webkit2_41)
fi

# --- One-off production build -------------------------------------------------
if [[ "${1:-}" == "--build" ]]; then
	shift
	echo ">>> wails build ${TAGS[*]:-}"
	wails build "${TAGS[@]}" "$@" || exit $?
	echo ">>> launching ./build/bin/sourdot"
	exec ./build/bin/sourdot
fi

# --- Dev loop -----------------------------------------------------------------
# -assetdir points the dev server at the on-disk frontend instead of the
# embedded copy, which is what makes frontend edits hot-reload. The frontend is
# deliberately Node-free, so there's no install/build step to run first.
echo ">>> wails dev ${TAGS[*]:-} -assetdir frontend/dist"
echo ">>> edit Go files to trigger a rebuild; edit frontend/dist to hot-reload"
echo ">>> Ctrl-C to stop"
echo
wails dev "${TAGS[@]}" -assetdir frontend/dist "$@"
