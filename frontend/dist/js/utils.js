// Small shared helpers used by every view.

function escapeHtml(str) {
    const div = document.createElement("div");
    div.textContent = str ?? "";
    return div.innerHTML;
}

function formatBytes(bytes) {
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
function installedLabel(v) {
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
function declaredLabel(p) {
    if (p.declaredVersion) return `${p.declaredVersion}.x`;
    if (p.detectedVersion) return `${p.detectedVersion}.x`;
    return "";
}

function formatDate(iso) {
    if (!iso) return "";
    try {
        return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
    } catch {
        return iso;
    }
}
