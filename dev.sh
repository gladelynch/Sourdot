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
# It detaches by default: there is no terminal window to keep around and no
# foreground job tying up the shell you started it from. The app's own output
# already goes to sourdot.log (see internal/applog); this script sends the
# build output `wails dev` writes -- compile errors, reload notices -- to
# dev.log in the same directory, so nothing is lost by having no tty.
#
# Usage:
#   ./dev.sh              # start detached (also what double-clicking does)
#   ./dev.sh --stop       # stop it
#   ./dev.sh --restart    # stop it, start it again
#   ./dev.sh --status     # is it running, and where are the logs
#   ./dev.sh --log        # follow dev.log
#   ./dev.sh --fg         # run in the foreground instead; Ctrl-C to stop
#   ./dev.sh --build      # one-off production build, then launch the binary
#
# Any other arguments are passed straight through to `wails dev`.

set -uo pipefail

cd "$(dirname "$(readlink -f "$0")")" || exit 1

# --- Where the detached process keeps its pid and output ----------------------
# Same directory the database and sourdot.log live in, so everything about a
# dev session is in one place; mirrors platform.ConfigDir().
STATE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/sourdot"
LOG="$STATE_DIR/dev.log"
PIDFILE="$STATE_DIR/dev.pid"

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

# -assetdir points the dev server at the on-disk frontend instead of the
# embedded copy, which is what makes frontend edits hot-reload. The frontend
# now goes through esbuild, but stays deliberately Node-free: the bundler is
# a Go program (go run ./tools/frontendbuild), so there's still nothing to
# npm-install first.
DEV_ARGS=("${TAGS[@]}" -assetdir frontend/dist)
WATCH_CMD="go run ./tools/frontendbuild -watch"

# build_frontend produces frontend/dist once, up front. The watcher would do
# this itself a moment later, but doing it here means wails dev never starts
# against a half-written dist, and a syntax error is reported before the
# window opens instead of into a detached log.
build_frontend() {
	if ! go run ./tools/frontendbuild; then
		echo "error: frontend build failed; fix the errors above and re-run." >&2
		return 1
	fi
}

# --- Process bookkeeping ------------------------------------------------------

# running echoes the pid of the detached dev process, or nothing. The cmdline
# check guards against a stale pidfile whose number has been reused by an
# unrelated process, which would otherwise make --stop kill a stranger.
running() {
	[[ -r "$PIDFILE" ]] || return 1
	local pid
	pid="$(<"$PIDFILE")"
	[[ "$pid" =~ ^[0-9]+$ ]] || return 1
	kill -0 "$pid" 2>/dev/null || return 1
	tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null | grep -q wails || return 1
	echo "$pid"
}

stop_dev() {
	local pid
	if ! pid="$(running)"; then
		rm -f "$PIDFILE"
		return 1
	fi
	# Negative pid signals the whole process group. setsid gave the process
	# its own group, so this also takes down the app binary `wails dev` built
	# and launched -- signalling only the parent would leave a stray window.
	kill -TERM -"$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
	for _ in {1..50}; do
		kill -0 "$pid" 2>/dev/null || break
		sleep 0.1
	done
	if kill -0 "$pid" 2>/dev/null; then
		kill -KILL -"$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null
	fi
	rm -f "$PIDFILE"
	return 0
}

start_dev() {
	local pid
	if pid="$(running)"; then
		echo "already running (pid $pid). Use --restart, or --log to watch it."
		return 0
	fi
	mkdir -p "$STATE_DIR" || exit 1

	# One previous run of history, same bargain applog strikes with sourdot.log.
	[[ -f "$LOG" ]] && mv -f "$LOG" "$LOG.1"

	build_frontend || return 1

	# The inner shell records its own $$ and then execs, so the pidfile holds
	# the real wails pid whether or not setsid chose to fork -- which depends
	# on whether the calling shell had job control on, and is not worth
	# guessing at from out here.
	#
	# The frontend watcher is started inside that same shell, before the exec,
	# so setsid's process group covers it too -- which is what lets --stop's
	# negative-pid kill take it down along with wails and the app binary,
	# instead of leaving an esbuild watcher running against a dead session.
	setsid bash -c 'echo $$ >"$1"; shift; '"${WATCH_CMD}"' & exec "$@"' \
		_ "$PIDFILE" wails dev -nocolour "${DEV_ARGS[@]}" "$@" \
		>>"$LOG" 2>&1 </dev/null &
	disown 2>/dev/null

	# Give it long enough to fail loudly (missing deps, port in use) rather
	# than reporting success for a process that is already gone.
	sleep 1
	if ! pid="$(running)"; then
		echo "error: dev server exited immediately. Last lines of $LOG:" >&2
		tail -n 20 "$LOG" >&2
		rm -f "$PIDFILE"
		return 1
	fi
	echo "started detached (pid $pid)"
	echo "  build output: $LOG        (./dev.sh --log to follow)"
	echo "  app log:      $STATE_DIR/sourdot.log"
	echo "  stop with:    ./dev.sh --stop"
}

# --- Subcommands --------------------------------------------------------------
case "${1:-}" in
--stop)
	if stop_dev; then echo "stopped."; else echo "not running."; fi
	exit 0
	;;
--restart)
	shift
	stop_dev >/dev/null && echo "stopped."
	start_dev "$@"
	exit $?
	;;
--status)
	if pid="$(running)"; then
		echo "running (pid $pid)"
	else
		echo "not running"
	fi
	echo "  build output: $LOG"
	echo "  app log:      $STATE_DIR/sourdot.log"
	exit 0
	;;
--log)
	[[ -f "$LOG" ]] || {
		echo "no log yet at $LOG" >&2
		exit 1
	}
	exec tail -n 50 -f "$LOG"
	;;
--build)
	shift
	# Double-clicked builds have nowhere to print to, so keep the output.
	if [[ ! -t 1 ]]; then
		mkdir -p "$STATE_DIR"
		exec >>"$LOG" 2>&1
	fi
	# wails build runs frontend:build (the bundler) from wails.json first,
	# so there's no separate frontend step to run here.
	echo ">>> wails build ${TAGS[*]:-}"
	wails build "${TAGS[@]}" "$@" || exit $?
	echo ">>> launching ./build/bin/sourdot"
	exec ./build/bin/sourdot
	;;
--fg)
	shift
	build_frontend || exit 1
	echo ">>> wails dev ${DEV_ARGS[*]}"
	echo ">>> $WATCH_CMD"
	echo ">>> edit Go files to trigger a rebuild; edit frontend/src to re-bundle"
	echo ">>> Ctrl-C to stop"
	echo
	# Ctrl-C reaches both: an interactive shell puts them in one foreground
	# process group and SIGINT goes to the group, not just the last command.
	$WATCH_CMD &
	exec wails dev "${DEV_ARGS[@]}" "$@"
	;;
*)
	start_dev "$@"
	exit $?
	;;
esac
