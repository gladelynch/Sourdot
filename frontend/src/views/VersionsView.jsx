// Versions view: one page holding both the installed versions (pinned at
// the top) and the full upstream catalog below, grouped into collapsible
// version series and covering every channel — stable, rc, beta, alpha and
// dev. Release notes and changelogs open in the user's real browser.
import { useState, useMemo, useCallback } from "preact/hooks";
import { store, actions } from "../store.js";
import { api } from "../api.js";
import { useResource, useAsyncAction, useInstallProgress } from "../hooks.js";
import { ProgressBar, EmptyState, Badge } from "../components/common.jsx";
import { MenuButton } from "../components/MenuButton.jsx";
import { formatBytes, formatDate, installedKey, failAlert } from "../utils.js";

// How many series are expanded on first paint. The catalog runs to ~60
// series, so everything past the newest few starts collapsed.
const DEFAULT_EXPANDED_GROUPS = 2;

const CHANNEL_FILTERS = {
    all: () => true,
    stable: (c) => c === "stable",
    prerelease: (c) => c === "rc" || c === "beta" || c === "alpha",
    dev: (c) => c === "dev",
};

const CHANNEL_CHIPS = [
    ["all", "All"],
    ["stable", "Stable"],
    ["prerelease", "RC / Beta"],
    ["dev", "Dev"],
];

export function VersionsView() {
    const catalog = useResource("catalog");
    const installed = useResource("installed");
    const defaultID = useResource("defaultVersionID");

    const [channel, setChannel] = useState("all");
    const [query, setQuery] = useState("");
    // Two sets rather than one: a series is expanded by default based on
    // its position, so we have to record explicit opens *and* explicit
    // closes to tell "never touched" from "deliberately shut".
    const [expanded, setExpanded] = useState(() => new Set());
    const [collapsed, setCollapsed] = useState(() => new Set());
    // What's being installed right now, as a label for the banner -- null
    // when nothing is. Doubles as the "freeze the lists" flag.
    const [installLabel, setInstallLabel] = useState(null);

    const releasesByTag = useMemo(() => {
        const map = new Map();
        for (const group of catalog) {
            for (const rel of group.releases || []) map.set(rel.tagName, rel);
        }
        return map;
    }, [catalog]);

    const installedByKey = useMemo(
        () => new Map(installed.map((v) => [installedKey(v.tagName, v.isMono), v])),
        [installed],
    );

    const [refreshCatalog, refreshing] = useAsyncAction(
        "Couldn't refresh the version catalog",
        () => actions.refreshCatalog(),
    );

    const [removeVersion, removing] = useAsyncAction(
        "Failed to remove version",
        (id) => actions.removeVersion(id),
    );

    const [setDefault] = useAsyncAction(
        "Failed to set default version",
        (id) => actions.setDefaultVersion(id),
    );

    // Release notes and changelogs are handed to the OS browser rather
    // than rendered in-app; the Go side allowlists the target host before
    // opening anything.
    const openNotes = useCallback(
        async (tagName, field) => {
            const url = releasesByTag.get(tagName)?.[field];
            if (!url) return;
            try {
                await api.openURL(url);
            } catch (err) {
                failAlert(`Couldn't open ${url}`, err);
            }
        },
        [releasesByTag],
    );

    const installVersion = useCallback(async (tagName, isMono) => {
        const label = `${tagName}${isMono ? " (.NET)" : ""}`;
        setInstallLabel(label);
        try {
            await actions.installVersion(tagName, isMono);
        } catch (err) {
            failAlert("Install failed", err);
        } finally {
            setInstallLabel(null);
        }
    }, []);

    const busy = installLabel !== null || removing;
    const progress = useInstallProgress(installLabel !== null, installLabel || "", `Installing ${installLabel}…`);

    const toggleSeries = useCallback((series, isOpen) => {
        if (isOpen) {
            setExpanded((prev) => remove(prev, series));
            setCollapsed((prev) => add(prev, series));
        } else {
            setCollapsed((prev) => remove(prev, series));
            setExpanded((prev) => add(prev, series));
        }
    }, []);

    const searching = query.trim() !== "";
    const groups = useMemo(() => {
        const q = query.trim().toLowerCase();
        const matchesChannel = CHANNEL_FILTERS[channel] || CHANNEL_FILTERS.all;
        const out = [];
        for (const group of catalog) {
            const releases = (group.releases || []).filter(
                (rel) => matchesChannel(rel.channel) && (!q || rel.tagName.toLowerCase().includes(q)),
            );
            if (releases.length) out.push({ ...group, releases });
        }
        return out;
    }, [catalog, channel, query]);

    const isExpanded = (series, index) => {
        if (collapsed.has(series)) return false;
        if (expanded.has(series)) return true;
        if (searching) return true; // typing a filter means you want to see the hits
        return index < DEFAULT_EXPANDED_GROUPS;
    };

    return (
        <section
            id="view-versions"
            class={`view is-active${busy ? " is-busy" : ""}`}
            aria-labelledby="view-versions-heading"
        >
            <header class="view-header">
                <h1 id="view-versions-heading">Versions</h1>
                <button class="btn" type="button" disabled={refreshing} onClick={() => refreshCatalog()}>
                    {refreshing ? "Refreshing…" : "Refresh"}
                </button>
            </header>

            {progress && (
                <div class="progress-banner" role="status">
                    <div class="progress-banner-label" id="install-progress-label">
                        {progress.label}
                    </div>
                    <ProgressBar
                        percent={progress.percent}
                        indeterminate={progress.indeterminate}
                        labelledBy="install-progress-label"
                    />
                </div>
            )}

            <section class="installed-shelf" aria-labelledby="installed-heading">
                <div class="section-bar">
                    <h2 id="installed-heading">Installed</h2>
                    <span class="muted">{installed.length ? `${installed.length} on disk` : ""}</span>
                </div>
                <InstalledShelf
                    installed={installed}
                    defaultID={defaultID}
                    releasesByTag={releasesByTag}
                    onNotes={openNotes}
                    onDefault={setDefault}
                    onRemove={removeVersion}
                />
            </section>

            <div class="available-toolbar">
                <div class="chip-group" role="group" aria-label="Filter by release channel">
                    {CHANNEL_CHIPS.map(([value, label]) => (
                        <button
                            key={value}
                            class={`chip${channel === value ? " is-active" : ""}`}
                            type="button"
                            aria-pressed={channel === value}
                            onClick={() => setChannel(value)}
                        >
                            {label}
                        </button>
                    ))}
                </div>
                <input
                    class="toolbar-search"
                    type="search"
                    placeholder="Filter by version…"
                    aria-label="Filter available versions"
                    autocomplete="off"
                    value={query}
                    onInput={(e) => setQuery(e.currentTarget.value)}
                />
            </div>

            <div aria-live="polite">
                {!store.isLoaded("catalog") ? (
                    <p class="muted">Loading available versions…</p>
                ) : groups.length === 0 ? (
                    <EmptyState>
                        <p>No versions match this filter.</p>
                    </EmptyState>
                ) : (
                    groups.map((group, i) => {
                        const open = isExpanded(group.series, i);
                        return (
                            <SeriesGroup
                                key={group.series}
                                group={group}
                                open={open}
                                installedByKey={installedByKey}
                                busy={busy}
                                onToggle={() => toggleSeries(group.series, open)}
                                onNotes={openNotes}
                                onInstall={installVersion}
                            />
                        );
                    })
                )}
            </div>
        </section>
    );
}

function InstalledShelf({ installed, defaultID, releasesByTag, onNotes, onDefault, onRemove }) {
    if (installed.length === 0) {
        return (
            <EmptyState>
                <p>No Godot versions installed yet.</p>
                <p class="muted">Pick one from the catalog below to install it.</p>
            </EmptyState>
        );
    }

    // Deliberately not re-sorted here: the backend already returns these
    // newest-and-most-stable first, using the same label ranking the
    // catalog below is ordered by. Sorting again in JS can only disagree
    // with it -- comparing labels as plain strings, as this used to, puts
    // dev2 ahead of dev3 and beta ahead of rc.
    return installed.map((v) => {
        const isDefault = v.id === defaultID;
        const rel = releasesByTag.get(v.tagName);
        return (
            <div class="version-row" key={v.id}>
                <div class="version-row-main">
                    <span class="version-row-name">
                        {v.tagName}
                        {v.isMono && <Badge>.NET</Badge>}
                        {isDefault && <Badge kind="badge-accent">default</Badge>}
                    </span>
                    <span class="muted">
                        {v.os}/{v.arch} · {formatBytes(v.sizeBytes)}
                    </span>
                </div>
                <div class="version-row-actions">
                    {rel?.releaseNotesURL && (
                        <button class="btn btn-sm btn-link" type="button" onClick={() => onNotes(v.tagName, "releaseNotesURL")}>
                            Notes ↗
                        </button>
                    )}
                    {!isDefault && (
                        <button class="btn btn-sm" type="button" onClick={() => onDefault(v.id)}>
                            Set as default
                        </button>
                    )}
                    <button class="btn btn-sm btn-danger-ghost" type="button" onClick={() => onRemove(v.id)}>
                        Uninstall
                    </button>
                </div>
            </div>
        );
    });
}

function SeriesGroup({ group, open, installedByKey, busy, onToggle, onNotes, onInstall }) {
    const installedCount = group.releases.filter(
        (rel) =>
            installedByKey.has(installedKey(rel.tagName, false)) ||
            installedByKey.has(installedKey(rel.tagName, true)),
    ).length;

    return (
        <section class={`series-group${open ? " is-open" : ""}`}>
            {/* Not disabled while busy: collapsing a series can't race an
                install, and freezing the whole page mid-download made the
                catalog feel broken. */}
            <button class="series-header" type="button" aria-expanded={open} onClick={onToggle}>
                <span class="series-caret" aria-hidden="true">▶</span>
                <span class="series-name">{group.series}</span>
                <Badge kind={`channel-${group.channel}`}>{group.channel}</Badge>
                {installedCount > 0 && <Badge kind="badge-installed">{installedCount} installed</Badge>}
                <span class="series-count muted">
                    {group.releases.length} {group.releases.length === 1 ? "release" : "releases"}
                </span>
            </button>
            {open && (
                <div class="series-body">
                    {group.releases.map((rel) => (
                        <ReleaseRow
                            key={rel.tagName}
                            rel={rel}
                            installedByKey={installedByKey}
                            busy={busy}
                            onNotes={onNotes}
                            onInstall={onInstall}
                        />
                    ))}
                </div>
            )}
        </section>
    );
}

// ReleaseRow is install-only. Uninstalling lives in the installed shelf at
// the top of the page, where there is already one row per installed build.
//
// The two variants used to sit in this row as a pair of buttons that
// switched between Install and Uninstall independently, which meant the row
// had to express four states across two controls. The common case -- one
// variant installed, the other not -- came out as a bright "Install" next
// to a quiet "Uninstall .NET", and read as though nothing was installed at
// all. Now the catalog only ever offers to get you something, the shelf
// only ever manages what you have, and which variant you already own shows
// up in two places that agree: the badges here, and the disabled entries in
// the menu.
function ReleaseRow({ rel, installedByKey, busy, onNotes, onInstall }) {
    const std = installedByKey.get(installedKey(rel.tagName, false));
    const mono = installedByKey.get(installedKey(rel.tagName, true));
    const hasStdAsset = (rel.assets || []).some((a) => !a.isMono);
    const hasMonoAsset = (rel.assets || []).some((a) => a.isMono);

    const variants = [
        { key: "standard", label: "Standard", available: hasStdAsset, installed: !!std, isMono: false },
        { key: "mono", label: ".NET", available: hasMonoAsset, installed: !!mono, isMono: true },
    ].filter((v) => v.available);

    const items = variants.map((v) => ({
        key: v.key,
        label: v.label,
        hint: v.installed ? "installed" : null,
        disabled: v.installed || busy,
        onSelect: () => onInstall(rel.tagName, v.isMono),
    }));

    // Nothing left to fetch: the row keeps its badges and its links, and
    // drops the action entirely rather than showing a dead control.
    const installable = variants.some((v) => !v.installed);

    return (
        <div class={`release-row${std || mono ? " is-installed" : ""}`}>
            <div class="release-row-id">
                <span class="release-row-tag">{rel.tagName}</span>
                <Badge kind={`channel-${rel.channel}`}>{rel.channel}</Badge>
                {/* One badge per installed variant, named. A single generic
                    "installed" couldn't say which of the two you had. */}
                {std && <Badge kind="badge-installed">Standard</Badge>}
                {mono && <Badge kind="badge-installed">.NET</Badge>}
            </div>
            <span class="release-row-date muted">{formatDate(rel.publishedAt)}</span>
            <div class="release-row-actions">
                {rel.releaseNotesURL && (
                    <button class="btn btn-sm btn-link" type="button" onClick={() => onNotes(rel.tagName, "releaseNotesURL")}>
                        Notes ↗
                    </button>
                )}
                {rel.changelogURL && (
                    <button class="btn btn-sm btn-link" type="button" onClick={() => onNotes(rel.tagName, "changelogURL")}>
                        Changelog ↗
                    </button>
                )}
                {installable && (
                    <MenuButton
                        label="Install"
                        menuLabel={`Install Godot ${rel.tagName}`}
                        disabled={busy}
                        items={items}
                    />
                )}
            </div>
        </div>
    );
}

// Set updates have to produce a new Set for Preact to see the change.
const add = (set, v) => new Set(set).add(v);
const remove = (set, v) => {
    const next = new Set(set);
    next.delete(v);
    return next;
};
