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

function formatDate(iso) {
    if (!iso) return "";
    try {
        return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
    } catch {
        return iso;
    }
}
