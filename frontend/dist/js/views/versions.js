// Versions view: one page holding both the installed versions (pinned at
// the top) and the full upstream catalog below, grouped into collapsible
// version series and covering every channel — stable, rc, beta, alpha and
// dev. Release notes and changelogs open in the user's real browser.

// How many series are expanded on first paint. The catalog runs to ~60
// series, so everything past the newest few starts collapsed.
const DEFAULT_EXPANDED_GROUPS = 2;

const CHANNEL_FILTERS = {
    all: () => true,
    stable: (c) => c === "stable",
    prerelease: (c) => c === "rc" || c === "beta" || c === "alpha",
    dev: (c) => c === "dev",
};

const versionsView = {
    state: {
        groups: [],
        releasesByTag: new Map(),
        installedByKey: new Map(), // `${tagName}|${isMono}` -> InstalledVersion
        installed: [],
        defaultID: "",
        channel: "all",
        query: "",
        // Two sets rather than one: a series is expanded by default based
        // on its position, so we have to record explicit opens *and*
        // explicit closes to tell "never touched" from "deliberately shut".
        expanded: new Set(),
        collapsed: new Set(),
        loaded: false,
        busy: false,
    },

    async init() {
        document.getElementById("refresh-versions-btn")
            .addEventListener("click", () => this.refresh());

        for (const chip of document.querySelectorAll(".available-toolbar .chip")) {
            chip.addEventListener("click", () => this.setChannel(chip.dataset.channel));
        }

        const search = document.getElementById("version-search");
        search.addEventListener("input", () => {
            this.state.query = search.value;
            this.renderAvailable();
        });

        // One delegated listener for the whole catalog: expanded series can
        // put hundreds of buttons on the page, and they're re-rendered on
        // every filter change.
        document.getElementById("available-versions-list")
            .addEventListener("click", (e) => this.onCatalogClick(e));
        document.getElementById("installed-versions-list")
            .addEventListener("click", (e) => this.onInstalledClick(e));

        events.on("download_progress", (evt) => this.onDownloadProgress(evt));
        events.on("checksum_verified", (evt) => this.onChecksumVerified(evt));
        events.on("install_complete", () => this.onInstallComplete());

        await this.load();
    },

    async load() {
        await Promise.all([this.refreshInstalled(), this.loadAvailable(false)]);
    },

    async refresh() {
        await Promise.all([this.refreshInstalled(), this.loadAvailable(true)]);
    },

    // --- Data ---------------------------------------------------------

    async loadAvailable(force) {
        const btn = document.getElementById("refresh-versions-btn");
        btn.disabled = true;
        btn.textContent = force ? "Refreshing…" : "Refresh";
        try {
            const groups = (force
                ? await api.refreshAvailableVersions()
                : await api.listAvailableVersions()) || [];
            this.state.groups = groups;
            this.state.releasesByTag = new Map();
            for (const group of groups) {
                for (const rel of group.releases || []) {
                    this.state.releasesByTag.set(rel.tagName, rel);
                }
            }
            this.state.loaded = true;
            this.renderAvailable();
        } catch (err) {
            document.getElementById("available-versions-list").innerHTML = `
                <div class="empty-state">
                    <p>Couldn't load the version catalog.</p>
                    <p class="muted">${escapeHtml(String(err))}</p>
                </div>`;
        } finally {
            btn.disabled = false;
            btn.textContent = "Refresh";
        }
    },

    async refreshInstalled() {
        let installed = [];
        let defaultID = "";
        try {
            [installed, defaultID] = await Promise.all([
                api.listInstalledVersions().then((v) => v || []),
                api.getDefaultVersion(),
            ]);
        } catch (err) {
            console.error("failed to load installed versions:", err);
        }

        // Deliberately not re-sorted here: the backend already returns
        // these newest-and-most-stable first, using the same label ranking
        // the catalog below is ordered by. Sorting again in JS can only
        // disagree with it -- comparing labels as plain strings, as this
        // used to, puts dev2 ahead of dev3 and beta ahead of rc.
        this.state.installed = installed;
        this.state.defaultID = defaultID || "";
        this.state.installedByKey = new Map(
            installed.map((v) => [installedKey(v.tagName, v.isMono), v]));

        this.renderInstalled();
        if (this.state.loaded) this.renderAvailable(); // flip Install <-> Uninstall
    },

    // --- Installed shelf ----------------------------------------------

    renderInstalled() {
        const container = document.getElementById("installed-versions-list");
        const { installed, defaultID } = this.state;

        document.getElementById("installed-count").textContent =
            installed.length ? `${installed.length} on disk` : "";

        if (installed.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No Godot versions installed yet.</p>
                    <p class="muted">Pick one from the catalog below to install it.</p>
                </div>`;
            return;
        }

        container.innerHTML = installed.map((v) => {
            const isDefault = v.id === defaultID;
            const rel = this.state.releasesByTag.get(v.tagName);
            return `
                <div class="version-row">
                    <div class="version-row-main">
                        <span class="version-row-name">
                            ${escapeHtml(v.tagName)}
                            ${v.isMono ? '<span class="badge">.NET</span>' : ""}
                            ${isDefault ? '<span class="badge badge-accent">default</span>' : ""}
                        </span>
                        <span class="muted">${escapeHtml(v.os)}/${escapeHtml(v.arch)} · ${formatBytes(v.sizeBytes)}</span>
                    </div>
                    <div class="version-row-actions">
                        ${rel && rel.releaseNotesURL
                            ? `<button class="btn btn-sm btn-link" data-action="notes" data-tag="${escapeHtml(v.tagName)}" type="button">Notes ↗</button>`
                            : ""}
                        ${isDefault ? "" : `<button class="btn btn-sm" data-action="default" data-id="${escapeHtml(v.id)}" type="button">Set as default</button>`}
                        <button class="btn btn-sm btn-danger-ghost" data-action="remove" data-id="${escapeHtml(v.id)}" type="button">Remove</button>
                    </div>
                </div>`;
        }).join("");
    },

    async onInstalledClick(e) {
        const btn = e.target.closest("button[data-action]");
        if (!btn || this.state.busy) return;

        switch (btn.dataset.action) {
            case "notes":
                await this.openNotes(btn.dataset.tag, "releaseNotesURL");
                break;
            case "default":
                await this.setDefault(btn.dataset.id);
                break;
            case "remove":
                await this.removeVersion(btn.dataset.id);
                break;
        }
    },

    // --- Available catalog --------------------------------------------

    visibleGroups() {
        const query = this.state.query.trim().toLowerCase();
        const matchesChannel = CHANNEL_FILTERS[this.state.channel] || CHANNEL_FILTERS.all;

        const out = [];
        for (const group of this.state.groups) {
            const releases = (group.releases || []).filter((rel) =>
                matchesChannel(rel.channel) &&
                (!query || rel.tagName.toLowerCase().includes(query)));
            if (releases.length) out.push({ ...group, releases });
        }
        return out;
    },

    isExpanded(series, index, searching) {
        if (this.state.collapsed.has(series)) return false;
        if (this.state.expanded.has(series)) return true;
        if (searching) return true; // typing a filter means you want to see the hits
        return index < DEFAULT_EXPANDED_GROUPS;
    },

    renderAvailable() {
        const container = document.getElementById("available-versions-list");
        if (!this.state.loaded) {
            container.innerHTML = `<p class="muted">Loading available versions…</p>`;
            return;
        }

        const groups = this.visibleGroups();
        if (groups.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No versions match this filter.</p>
                </div>`;
            return;
        }

        const searching = this.state.query.trim() !== "";
        container.innerHTML = groups.map((group, i) => {
            const open = this.isExpanded(group.series, i, searching);
            const installedCount = group.releases
                .filter((rel) => this.isInstalled(rel.tagName)).length;

            return `
                <section class="series-group${open ? " is-open" : ""}">
                    <button class="series-header" type="button" data-action="toggle-series"
                            data-series="${escapeHtml(group.series)}" aria-expanded="${open}">
                        <span class="series-caret" aria-hidden="true">▶</span>
                        <span class="series-name">${escapeHtml(group.series)}</span>
                        <span class="badge channel-${escapeHtml(group.channel)}">${escapeHtml(group.channel)}</span>
                        ${installedCount ? `<span class="badge badge-installed">${installedCount} installed</span>` : ""}
                        <span class="series-count muted">${group.releases.length} ${group.releases.length === 1 ? "release" : "releases"}</span>
                    </button>
                    ${open ? `<div class="series-body">${group.releases.map((rel) => this.releaseRow(rel)).join("")}</div>` : ""}
                </section>`;
        }).join("");
    },

    releaseRow(rel) {
        const tag = escapeHtml(rel.tagName);
        const std = this.state.installedByKey.get(installedKey(rel.tagName, false));
        const mono = this.state.installedByKey.get(installedKey(rel.tagName, true));
        const hasStdAsset = (rel.assets || []).some((a) => !a.isMono);
        const hasMonoAsset = (rel.assets || []).some((a) => a.isMono);

        const variantButton = (installedVersion, hasAsset, label, isMono) => {
            if (!hasAsset) return "";
            if (installedVersion) {
                return `<button class="btn btn-sm btn-danger-ghost" data-action="remove"
                        data-id="${escapeHtml(installedVersion.id)}" type="button">Uninstall ${label}</button>`;
            }
            return `<button class="btn btn-sm${isMono ? "" : " btn-accent"}" data-action="install"
                    data-tag="${tag}" data-mono="${isMono}" type="button">Install${isMono ? " .NET" : ""}</button>`;
        };

        return `
            <div class="release-row${std || mono ? " is-installed" : ""}">
                <div class="release-row-id">
                    <span class="release-row-tag">${tag}</span>
                    <span class="badge channel-${escapeHtml(rel.channel)}">${escapeHtml(rel.channel)}</span>
                    ${std || mono ? '<span class="badge badge-installed">installed</span>' : ""}
                </div>
                <span class="release-row-date muted">${escapeHtml(formatDate(rel.publishedAt))}</span>
                <div class="release-row-actions">
                    ${rel.releaseNotesURL
                        ? `<button class="btn btn-sm btn-link" data-action="notes" data-tag="${tag}" type="button">Notes ↗</button>`
                        : ""}
                    ${rel.changelogURL
                        ? `<button class="btn btn-sm btn-link" data-action="changelog" data-tag="${tag}" type="button">Changelog ↗</button>`
                        : ""}
                    ${variantButton(std, hasStdAsset, "", false)}
                    ${variantButton(mono, hasMonoAsset, ".NET", true)}
                </div>
            </div>`;
    },

    async onCatalogClick(e) {
        const btn = e.target.closest("button[data-action]");
        if (!btn) return;

        if (btn.dataset.action === "toggle-series") {
            this.toggleSeries(btn.dataset.series);
            return;
        }
        if (this.state.busy) return;

        switch (btn.dataset.action) {
            case "notes":
                await this.openNotes(btn.dataset.tag, "releaseNotesURL");
                break;
            case "changelog":
                await this.openNotes(btn.dataset.tag, "changelogURL");
                break;
            case "install":
                await this.installVersion(btn.dataset.tag, btn.dataset.mono === "true");
                break;
            case "remove":
                await this.removeVersion(btn.dataset.id);
                break;
        }
    },

    toggleSeries(series) {
        const group = document.querySelector(`.series-header[data-series="${CSS.escape(series)}"]`);
        const isOpen = group?.getAttribute("aria-expanded") === "true";
        if (isOpen) {
            this.state.expanded.delete(series);
            this.state.collapsed.add(series);
        } else {
            this.state.collapsed.delete(series);
            this.state.expanded.add(series);
        }
        this.renderAvailable();
    },

    setChannel(channel) {
        this.state.channel = channel;
        for (const chip of document.querySelectorAll(".available-toolbar .chip")) {
            const active = chip.dataset.channel === channel;
            chip.classList.toggle("is-active", active);
            chip.setAttribute("aria-pressed", String(active));
        }
        this.renderAvailable();
    },

    isInstalled(tagName) {
        return this.state.installedByKey.has(installedKey(tagName, false)) ||
            this.state.installedByKey.has(installedKey(tagName, true));
    },

    // --- Actions ------------------------------------------------------

    // Release notes and changelogs are handed to the OS browser rather than
    // rendered in-app; the Go side allowlists the target host before
    // opening anything.
    async openNotes(tagName, field) {
        const rel = this.state.releasesByTag.get(tagName);
        const url = rel && rel[field];
        if (!url) return;
        try {
            await api.openURL(url);
        } catch (err) {
            alert(`Couldn't open ${url}:\n${err}`);
        }
    },

    async installVersion(tagName, isMono) {
        this.setBusy(true);
        this.showProgress(`Installing ${tagName}${isMono ? " (.NET)" : ""}…`);
        try {
            await api.installVersion(tagName, isMono);
        } catch (err) {
            this.hideProgress();
            this.setBusy(false);
            alert(`Install failed: ${err}`);
        }
        // On success, onInstallComplete (fired via the install_complete
        // event) hides the banner and refreshes the lists.
    },

    async removeVersion(id) {
        this.setBusy(true);
        try {
            await api.removeVersion(id);
        } catch (err) {
            alert(`Failed to remove version: ${err}`);
        }
        this.setBusy(false);
        await this.refreshInstalled();
    },

    async setDefault(id) {
        try {
            await api.setDefaultVersion(id);
        } catch (err) {
            alert(`Failed to set default version: ${err}`);
        }
        await this.refreshInstalled();
    },

    setBusy(busy) {
        this.state.busy = busy;
        document.getElementById("view-versions").classList.toggle("is-busy", busy);
    },

    // --- Progress banner ----------------------------------------------

    showProgress(label) {
        document.getElementById("install-progress-label").textContent = label;
        document.getElementById("install-progress-fill").style.width = "0%";
        document.getElementById("install-progress").hidden = false;
    },

    hideProgress() {
        document.getElementById("install-progress").hidden = true;
    },

    onDownloadProgress(evt) {
        const data = evt?.data;
        if (!data || !data.total) return;
        const pct = Math.min(100, Math.round((data.downloaded / data.total) * 100));
        const fill = document.getElementById("install-progress-fill");
        if (fill) fill.style.width = `${pct}%`;
        const label = document.getElementById("install-progress-label");
        if (label) label.textContent = `Downloading… ${pct}%`;
    },

    onChecksumVerified(evt) {
        const label = document.getElementById("install-progress-label");
        if (!label) return;
        label.textContent = evt?.data?.verified ? "Verified — extracting…" : "Extracting… (checksum not published for this release)";
    },

    async onInstallComplete() {
        this.hideProgress();
        this.setBusy(false);
        await this.refreshInstalled();
    },
};

// installedKey pairs a release tag with its standard/mono variant, since
// both can be installed side by side from the same release.
function installedKey(tagName, isMono) {
    return `${tagName}|${isMono ? "mono" : "standard"}`;
}
