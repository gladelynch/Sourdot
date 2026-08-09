// Projects view: add/remove tracked projects, favorite/tag them, pin a
// specific engine version, and open a project (auto-installing its
// resolved version first if needed).
//
// Rendered as a plain DOM list rather than a virtualized one -- correct at
// the scale a single developer's project list realistically reaches;
// revisit if that stops being true.
const projectsView = {
    installedVersions: [],

    async init() {
        document.getElementById("add-project-btn").addEventListener("click", () => this.addProject());
        await this.refresh();
    },

    async refresh() {
        const container = document.getElementById("projects-list");
        let projects = [];
        try {
            [projects, this.installedVersions] = await Promise.all([
                api.listProjects().then((p) => p || []),
                api.listInstalledVersions().then((v) => v || []),
            ]);
        } catch (err) {
            console.error("failed to load projects:", err);
        }
        this.render(container, projects);
    },

    render(container, projects) {
        if (projects.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <p>No projects tracked yet.</p>
                    <p class="muted">Click "Add project…" to track one.</p>
                </div>`;
            return;
        }

        const sorted = [...projects].sort((a, b) => {
            if (a.favorite !== b.favorite) return a.favorite ? -1 : 1;
            return a.name.localeCompare(b.name);
        });

        container.innerHTML = "";
        const list = document.createElement("div");
        list.className = "project-list";
        for (const p of sorted) {
            list.appendChild(this.renderRow(p));
        }
        container.appendChild(list);
    },

    renderRow(p) {
        const row = document.createElement("div");
        row.className = "project-row";
        row.dataset.id = p.id;

        const versionOptions = this.installedVersions
            .map((v) => {
                const selected = v.id === p.pinnedVersionId ? "selected" : "";
                const label = `${v.version}${v.isMono ? " (mono)" : ""}`;
                return `<option value="${escapeHtml(v.id)}" ${selected}>${escapeHtml(label)}</option>`;
            })
            .join("");

        row.innerHTML = `
            <div class="project-row-main">
                <button class="star-btn ${p.favorite ? "is-favorite" : ""}" data-action="favorite" type="button" title="Favorite">★</button>
                <div class="project-row-info">
                    <div class="project-row-name">
                        ${escapeHtml(p.name)}
                        ${p.usesCSharp ? '<span class="badge">C#</span>' : ""}
                        ${p.detectedVersion ? `<span class="badge">Godot ${escapeHtml(p.detectedVersion)}</span>` : ""}
                        ${p.missing ? '<span class="badge badge-danger">missing</span>' : ""}
                    </div>
                    <div class="muted project-row-path">${escapeHtml(p.path)}</div>
                    <input class="tags-input" data-action="tags" type="text" placeholder="tags, comma, separated" value="${escapeHtml((p.tags || []).join(", "))}" />
                </div>
            </div>
            <div class="project-row-actions">
                <select class="version-select" data-action="pin" title="Pinned version">
                    <option value="">(auto-detect)</option>
                    ${versionOptions}
                </select>
                <button class="btn btn-sm btn-accent" data-action="open" type="button">Open</button>
                <button class="btn btn-sm" data-action="remove" type="button">Remove</button>
            </div>
            <div class="project-row-status muted" data-status hidden></div>`;

        row.querySelector('[data-action="favorite"]').addEventListener("click", () => this.toggleFavorite(p));
        row.querySelector('[data-action="tags"]').addEventListener("change", (e) => this.saveTags(p, e.target.value));
        row.querySelector('[data-action="pin"]').addEventListener("change", (e) => this.setPin(p, e.target.value));
        row.querySelector('[data-action="open"]').addEventListener("click", () => this.openProject(p, row));
        row.querySelector('[data-action="remove"]').addEventListener("click", () => this.removeProject(p));

        return row;
    },

    async addProject() {
        try {
            const proj = await api.pickAndAddProject();
            if (!proj) return; // user cancelled the folder picker
        } catch (err) {
            alert(`Failed to add project: ${err}`);
            return;
        }
        await this.refresh();
    },

    async toggleFavorite(p) {
        try {
            await api.setFavorite(p.id, !p.favorite);
        } catch (err) {
            alert(`Failed to update favorite: ${err}`);
        }
        await this.refresh();
    },

    async saveTags(p, value) {
        const tags = value.split(",").map((t) => t.trim()).filter(Boolean);
        try {
            await api.setTags(p.id, tags);
        } catch (err) {
            alert(`Failed to save tags: ${err}`);
            await this.refresh();
        }
    },

    async setPin(p, versionId) {
        try {
            await api.setPinnedVersion(p.id, versionId);
        } catch (err) {
            alert(`Failed to set pinned version: ${err}`);
            await this.refresh();
        }
    },

    async openProject(p, row) {
        const status = row.querySelector("[data-status]");
        status.hidden = false;
        status.textContent = "Opening… (installing the engine version first, if needed — see Versions for progress)";
        try {
            await api.openProject(p.id);
        } catch (err) {
            alert(`Failed to open project: ${err}`);
        }
        status.hidden = true;
        await this.refresh(); // pick up the updated "last opened" state
    },

    async removeProject(p) {
        if (!confirm(`Stop tracking "${p.name}"? This won't delete any files.`)) return;
        try {
            await api.removeProject(p.id);
        } catch (err) {
            alert(`Failed to remove project: ${err}`);
        }
        await this.refresh();
    },
};
