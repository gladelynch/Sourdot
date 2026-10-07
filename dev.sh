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
#   ./dev.sh --gui ...    # no terminal: output goes to dev.log, a console
#                         # window shows it until the app opens, and failures
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

# dev.log and dev.pid live next to the database and sourdot.log (mirrors
# platform.ConfigDir()).
CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/sourdot"
mkdir -p "$CONFIG"
PIDFILE="$CONFIG/dev.pid"
LOG="$CONFIG/dev.log"

# plain strips the colour codes wails puts in its output, and squeezes the
# long runs of spaces its spinner leaves behind, for showing in a dialog.
plain() {
	sed -u -E 's/\x1b\[[0-9;]*[A-Za-z]//g; s/ {8,}/ /g'
}

# dialog shows a message box: `dialog error TEXT`, or `dialog question TEXT`,
# whose exit status is the answer (no dialog tool means "no").
dialog() {
	local kind="$1" text="$2"
	if command -v zenity >/dev/null 2>&1; then
		zenity "--$kind" --no-markup --title "Sourdot (dev)" --width 700 --text "$text" 2>/dev/null
	elif command -v kdialog >/dev/null 2>&1; then
		[[ "$kind" == question ]] && kind=yesno
		kdialog --title "Sourdot (dev)" "--$kind" "$text"
	else
		command -v notify-send >/dev/null 2>&1 && notify-send -u critical "Sourdot (dev)" "$text"
		return 1
	fi
}

# The console is a live view of dev.log, shown under --gui while there's
# nothing else on screen: installing wails the first time, then building. It
# closes itself once the app window opens. Hide just closes it; Stop, or
# closing the window, cancels the launch. Without zenity there's no live view,
# only a notification.
CONSOLE=
console_open() {
	if ! command -v zenity >/dev/null 2>&1; then
		command -v notify-send >/dev/null 2>&1 &&
			notify-send "Sourdot (dev)" "Starting; the app will open when it's built. Output: $LOG"
		return
	fi
	(
		# Not a pipeline: `wait` on one waits for all of it, and tail never ends.
		zenity --text-info --auto-scroll --title "Sourdot (dev): starting" \
			--width 900 --height 500 --ok-label Hide --cancel-label Stop \
			< <(tail -n +1 -F "$LOG" 2>/dev/null | plain) 2>/dev/null &
		zenity=$!
		trap 'kill "$zenity" 2>/dev/null; exit 0' TERM
		wait "$zenity"
		(($? == 1)) && kill -TERM 0
	) &
	CONSOLE=$!
}
console_close() {
	[[ -n "$CONSOLE" ]] && kill "$CONSOLE" 2>/dev/null
	CONSOLE=
}

# fail reports an error and exits. With --gui nobody is watching the output,
# so it also shows a dialog with the end of the log, which is where the
# compiler error will be.
fail() {
	local output=
	((GUI)) && output="$(tail -n 25 "$LOG" | plain)"
	echo >&2
	echo "error: $*" >&2
	if ((GUI)); then
		console_close
		dialog error "$*"$'\n\n'"${output:+$output$'\n\n'}Full output: $LOG"
	fi
	exit 1
}

# --- One at a time ------------------------------------------------------------
# A second copy would fight the first over the dev server port. A --gui copy
# has no window of its own once the console is hidden, so if one gets stuck
# (the app crashed, and wails dev is waiting for a fix to rebuild) there's
# nothing to close; offer to stop it instead.
OTHER="$(cat "$PIDFILE" 2>/dev/null)"
if [[ -n "$OTHER" && "$OTHER" != "$$" ]] && kill -0 "$OTHER" 2>/dev/null &&
	ps -o args= -p "$OTHER" | grep -q 'dev\.sh'; then
	msg="Sourdot (dev) is already running (pid $OTHER)."
	if ((GUI)) && [[ "$(ps -o pgid= -p "$OTHER" | tr -d ' ')" == "$OTHER" ]]; then
		dialog question "$msg"$'\n\n'"Stop it and start again?" || exit 0
		kill -TERM -- "-$OTHER" 2>/dev/null
		for _ in {1..40}; do
			kill -0 "$OTHER" 2>/dev/null || break
			sleep 0.25
		done
	else
		echo "error: $msg Stop it first." >&2
		((GUI)) && dialog error "$msg Stop it first."
		exit 1
	fi
fi
echo $$ >"$PIDFILE"

# When this script exits, for any reason, take everything it started down with
# it: the console, the frontend watcher, wails and the app. `kill 0` signals
# this script's whole process group; the trap is cleared first so it doesn't
# run twice. From a terminal that group can include whatever ran this script
# (make, say), so it's only done once there's something in the background to
# stop; under --gui the group is always ours (see setsid above).
STARTED=$GUI
cleanup() {
	trap - EXIT INT TERM HUP
	[[ "$(cat "$PIDFILE" 2>/dev/null)" == "$$" ]] && rm -f "$PIDFILE"
	((STARTED)) && kill 0 2>/dev/null
}
trap cleanup EXIT INT TERM HUP

# With no terminal there's nowhere for output to go, so keep it in dev.log.
if ((GUI)); then
	exec >"$LOG" 2>&1
	console_open
fi

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

# Linux distros that ship only webkit2gtk-4.1 need this build tag; see the
# Makefile and the README's "Building" section. Windows/macOS don't.
TAGS=()
if [[ "$(uname -s)" == "Linux" ]]; then
	TAGS=(-tags webkit2_41)

	# Wails links against GTK and WebKitGTK through cgo. Without their headers
	# the build fails deep inside wails dev, so check up front and say what to
	# install.
	missing=()
	command -v gcc >/dev/null 2>&1 || missing+=(gcc)
	command -v pkg-config >/dev/null 2>&1 || missing+=(pkg-config)
	for pc in gtk+-3.0 webkit2gtk-4.1; do
		pkg-config --exists "$pc" 2>/dev/null || missing+=("$pc")
	done
	((${#missing[@]} == 0)) ||
		fail "missing build dependencies: ${missing[*]}."$'\n\n'"Fedora: sudo dnf install -y gtk3-devel webkit2gtk4.1-devel gcc pkgconf-pkg-config"$'\n'"Debian/Ubuntu: sudo apt install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config"
fi

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
	echo ">>> wails not found; installing $WAILS_VERSION (one-time, takes a minute or two)"
	go install -v "github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION" ||
		fail "installing wails failed; see the output below."
	echo ">>> wails installed"
fi

# --- Run ----------------------------------------------------------------------

# Build frontend/dist once up front. The watcher would do this itself a moment
# later, but doing it here means wails dev never starts against a half-written
# dist, and a syntax error is reported before the window opens.
echo ">>> building the frontend"
go run ./tools/frontendbuild ||
	fail "frontend build failed; fix the errors and run this again."

STARTED=1
go run ./tools/frontendbuild -watch &

# -assetdir points the dev server at the on-disk frontend instead of the
# embedded copy, which is what makes frontend edits hot-reload.
echo ">>> wails dev ${TAGS[*]} -assetdir frontend/dist $*"
echo ">>> edit Go files to rebuild, frontend/src to re-bundle; Ctrl-C to stop"
echo
if ! ((GUI)); then
	wails dev "${TAGS[@]}" -assetdir frontend/dist "$@" ||
		fail "wails dev exited with an error; see the output above."
	exit 0
fi

# wails dev doesn't exit when the first build fails; it waits for a source
# change to retry. In a terminal that's handy, but here nobody would ever see
# the error, so watch its output for how the first build went. Failure is
# checked first: wails prints the dev server URL either way.
wails dev "${TAGS[@]}" -assetdir frontend/dist "$@" &
WAILS=$!
while :; do
	grep -qF "No version running" "$LOG" &&
		fail "building the app failed."
	grep -qF "Using DevServer URL" "$LOG" && break
	kill -0 "$WAILS" 2>/dev/null || break
	sleep 0.5
done
console_close
wait "$WAILS" ||
	fail "wails dev exited with an error."
