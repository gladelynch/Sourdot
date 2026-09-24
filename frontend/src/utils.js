// Small shared helpers used across the app.

// --- error reporting -------------------------------------------------
//
// console.* here only reaches the WebKit inspector, which a built app has
// no way to open -- so anything logged that way in a release build is
// simply lost. Everything below funnels frontend failures through the Go
// binding instead, landing them in the same sourdot.log as the backend's
// own output, interleaved in real time. console.* is kept alongside it so
// `wails dev` stays convenient.

// Wails injects window.go asynchronously, so the earliest errors -- which
// are the ones most worth having -- can arrive before the binding exists.
// They're queued rather than dropped, and flushed once it shows up.
const _pendingLogs = [];
let _logFlushTimer = null;

function _flushLogs() {
    const fn = window.go?.main?.App?.LogFrontend;
    if (!fn) return false;
    while (_pendingLogs.length) {
        const [level, message] = _pendingLogs.shift();
        try {
            fn(level, message);
        } catch {
            // The log must never become its own source of errors.
        }
    }
    if (_logFlushTimer) {
        clearInterval(_logFlushTimer);
        _logFlushTimer = null;
    }
    return true;
}

export function logToBackend(level, message) {
    _pendingLogs.push([level, String(message)]);
    if (_flushLogs()) return;
    // Give up after ~10s: if the binding never appears, the backend is gone
    // and there is nothing left to log to anyway.
    if (!_logFlushTimer) {
        let tries = 0;
        _logFlushTimer = setInterval(() => {
            if (_flushLogs() || ++tries > 100) {
                clearInterval(_logFlushTimer);
                _logFlushTimer = null;
            }
        }, 100);
    }
}

// describeError renders a thrown value for a log line. Errors carry a
// stack worth keeping; Go-side errors arrive as plain strings and don't.
//
// The message is composed explicitly rather than trusting err.stack, because
// WebKit -- which is the engine Sourdot actually ships on -- starts the
// stack at the first frame and omits the message entirely. Logging the raw
// stack there threw away the only part anyone reads. V8 does prepend it, so
// that case is detected instead of duplicated.
export function describeError(err) {
    if (!(err instanceof Error)) return String(err);
    const head = `${err.name}: ${err.message}`;
    if (!err.stack) return head;
    return err.stack.startsWith(head) ? err.stack : `${head}\n${err.stack}`;
}

// logError records a failure without interrupting the user.
export function logError(context, err) {
    console.error(`${context}:`, err);
    logToBackend("error", `${context}: ${describeError(err)}`);
}

// failAlert records a failure and tells the user about it. The alert text
// stays `${context}: ${err}` so the dialog reads the way it always has,
// while the log gets the stack too.
export function failAlert(context, err) {
    logError(context, err);
    alert(`${context}: ${err}`);
}

// installGlobalErrorHandlers catches what never reaches a try/catch --
// exactly the failures nobody would otherwise see. Called once from
// main.jsx rather than running as a module side effect, so importing a
// helper from here can't quietly install listeners.
export function installGlobalErrorHandlers() {
    window.addEventListener("error", (evt) => {
        const where = evt.filename ? ` (${evt.filename}:${evt.lineno}:${evt.colno})` : "";
        logToBackend("error", `uncaught: ${describeError(evt.error ?? evt.message)}${where}`);
    });

    window.addEventListener("unhandledrejection", (evt) => {
        logToBackend("error", `unhandled rejection: ${describeError(evt.reason)}`);
    });
}

// --- formatting ------------------------------------------------------
//
// escapeHtml used to live here, because every view built its markup as
// strings. JSX escapes interpolated values itself, so the helper is gone
// along with the class of bug it existed to prevent.

export function formatBytes(bytes) {
    if (!bytes) return "0 B";
    const units = ["B", "KB", "MB", "GB"];
    let n = bytes;
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
        n /= 1024;
        i++;
    }
    return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// installedLabel names an installed version the way the user picked it:
// the full release tag, so pre-release stages stay distinguishable. The
// numeric `version` field alone collapses 4.8-dev3, 4.8-rc1 and 4.8-stable
// into three identical "4.8.0" entries. tagName is backfilled by the
// backend for old records, so the `version` fallback is belt-and-braces.
export function installedLabel(v) {
    return `${v.tagName || v.version}${v.isMono ? " (.NET)" : ""}`;
}

// declaredLabel names the version a project records for itself, and does it
// as a series rather than as an exact version. project.godot's
// config/features holds "4.8" with no build label, and that means "the most
// recent 4.8" -- stable once the series ships, the latest pre-release until
// then. "4.8.x" is the honest rendering of that: a plain "4.8" would claim
// a precision the file doesn't have and hide, for the same reason as above,
// that 4.8-dev3, 4.8-beta1, 4.8-rc1 and 4.8-stable are four different
// builds. Godot 3 projects record no feature version at all, leaving only
// the major from config_version. Returns "" when even that is unknown.
export function declaredLabel(p) {
    if (p.declaredVersion) return `${p.declaredVersion}.x`;
    if (p.detectedVersion) return `${p.detectedVersion}.x`;
    return "";
}

export function formatDate(iso) {
    if (!iso) return "";
    try {
        return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
    } catch {
        return iso;
    }
}

// installedKey pairs a release tag with its standard/mono variant, since
// both can be installed side by side from the same release.
export function installedKey(tagName, isMono) {
    return `${tagName}|${isMono ? "mono" : "standard"}`;
}
