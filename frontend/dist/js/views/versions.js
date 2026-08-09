// Versions view: browse/install/remove Godot versions, with a changelog
// preview and a live install progress banner.
const versionsView = {
    async init() {
        document.getElementById("browse-versions-btn").addEventListener("click", () => this.openBrowsePanel());
        document.getElementById("browse-panel-close").addEventListener("click", () => this.closeBrowsePanel());
        document.getElementById("browse-panel").addEventListener("click", (e) => {
            if (e.target.id === "browse-panel") this.closeBrowsePanel(); // click on the overlay itself
        });

        events.on("download_progress", (evt) => this.onDownloadProgress(evt));
        events.on("checksum_verified", (evt) => this.onChecksumVerified(evt));
        events.on("install_complete", () => this.onInstallComplete());

        await this.refreshInstalled();
    },

    async refreshInstalled() {
        const container = document.getElementById("installed-versions-list");
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
        this.renderInstalled(container, installed, defaultID);
    },

    renderInstalled(container, versions, defaultID) {
        if (versions.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No Godot versions installed yet.</p>
                    <p class="muted">Click "Browse available…" to install one.</p>
                </div>`;
            return;
        }

        container.innerHTML = "";
        const list = document.createElement("div");
        list.className = "version-list";
        for (const v of versions) {
            const isDefault = v.id === defaultID;
            const row = document.createElement("div");
            row.className = "version-row";
            row.innerHTML = `
                <div class="version-row-main">
                    <span class="version-row-name">
                        ${escapeHtml(v.version)}
                        ${v.isMono ? '<span class="badge">mono</span>' : ""}
                        ${isDefault ? '<span class="badge badge-accent">default</span>' : ""}
                    </span>
                    <span class="muted">${escapeHtml(v.os)}/${escapeHtml(v.arch)} · ${formatBytes(v.sizeBytes)}</span>
                </div>
                <div class="version-row-actions">
                    ${isDefault ? "" : `<button class="btn btn-sm" data-action="default">Set as default</button>`}
                    <button class="btn btn-sm" data-action="remove">Remove</button>
                </div>`;
            row.querySelector('[data-action="remove"]')?.addEventListener("click", () => this.removeVersion(v.id));
            row.querySelector('[data-action="default"]')?.addEventListener("click", () => this.setDefault(v.id));
            list.appendChild(row);
        }
        container.appendChild(list);
    },

    async removeVersion(id) {
        try {
            await api.removeVersion(id);
        } catch (err) {
            alert(`Failed to remove version: ${err}`);
        }
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

    async openBrowsePanel() {
        const panel = document.getElementById("browse-panel");
        const container = document.getElementById("available-versions-list");
        panel.hidden = false;
        container.innerHTML = `<p class="muted">Loading releases…</p>`;
        try {
            const releases = (await api.listAvailableVersions()) || [];
            this.renderAvailable(container, releases);
        } catch (err) {
            container.innerHTML = `<p class="muted">Failed to load releases: ${escapeHtml(String(err))}</p>`;
        }
    },

    closeBrowsePanel() {
        document.getElementById("browse-panel").hidden = true;
    },

    renderAvailable(container, releases) {
        if (releases.length === 0) {
            container.innerHTML = `<p class="muted">No stable releases found.</p>`;
            return;
        }
        container.innerHTML = "";
        for (const rel of releases) {
            const row = document.createElement("div");
            row.className = "release-row";
            row.innerHTML = `
                <div class="release-row-header">
                    <span class="release-row-tag">${escapeHtml(rel.tagName)}</span>
                    <span class="muted">${formatDate(rel.publishedAt)}</span>
                    <button class="btn btn-sm" data-action="toggle-notes" type="button">Release notes</button>
                </div>
                <div class="release-row-actions">
                    <button class="btn btn-sm btn-accent" data-action="install-standard" type="button">Install</button>
                    <button class="btn btn-sm" data-action="install-mono" type="button">Install (Mono)</button>
                </div>
                <pre class="release-changelog" hidden>${escapeHtml(rel.bodyMD || "(no release notes)")}</pre>`;
            container.appendChild(row);

            row.querySelector('[data-action="toggle-notes"]').addEventListener("click", () => {
                const notes = row.querySelector(".release-changelog");
                notes.hidden = !notes.hidden;
            });
            row.querySelector('[data-action="install-standard"]').addEventListener("click", () => this.installVersion(rel, false));
            row.querySelector('[data-action="install-mono"]').addEventListener("click", () => this.installVersion(rel, true));
        }
    },

    async installVersion(rel, isMono) {
        this.closeBrowsePanel();
        this.showProgress(`Installing ${rel.tagName}${isMono ? " (mono)" : ""}…`);
        try {
            await api.installVersion(rel, isMono);
        } catch (err) {
            this.hideProgress();
            alert(`Install failed: ${err}`);
        }
        // On success, onInstallComplete (fired via the install_complete
        // event) hides the banner and refreshes the list.
    },

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
        await this.refreshInstalled();
    },
};
