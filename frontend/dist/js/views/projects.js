// Projects view: add/remove tracked projects, favorite/tag them, choose
// which engine version each one opens in, and open them.
//
// The version control here is deliberately not an "auto-detect" toggle.
// Opening a project in a newer editor makes Godot rewrite its files
// one-way, so a project only ever opens in a version something declared:
// its own project settings (the default for anything newly added), the
// app-wide default if the user opts this project into it, or a specific
// build they pinned. When the version a project needs isn't installed, the
// row says which one and offers to fetch it -- it never falls back to
// whatever happens to be lying around.
//
// Rendered as a plain DOM list rather than a virtualized one -- correct at
// the scale a single developer's project list realistically reaches;
// revisit if that stops being true.
const projectsView = {
    installedVersions: [],
    defaultVersion: null,

    async init() {
        document.getElementById("add-project-btn").addEventListener("click", () => this.addProject());
        await this.refresh();
    },

    async refresh() {
        const container = document.getElementById("projects-list");
        let projects = [];
        let defaultId = "";
        try {
            [projects, this.installedVersions, defaultId] = await Promise.all([
                api.listProjects().then((p) => p || []),
                api.listInstalledVersions().then((v) => v || []),
                api.getDefaultVersion().catch(() => ""),
            ]);
        } catch (err) {
            console.error("failed to load projects:", err);
        }
        this.defaultVersion = this.installedVersions.find((v) => v.id === defaultId) || null;
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

    // versionOptions builds the version menu: the project's own version
    // first (and selected, for anything not explicitly changed), then the
    // app-wide default, then every installed build. The first two spell out
    // the build they currently mean, so "Project's version" isn't a mystery
    // the user has to click Open to resolve.
    //
    // Every version here is named by its full release tag. A bare "4.8" is
    // never shown: it's four different builds (4.8-dev3, 4.8-beta1,
    // 4.8-rc1, 4.8-stable) wearing one name. Where only a series is known
    // -- project.godot records no build label at all, so "4.8" means
    // whichever 4.8 is current -- it's written "4.8.x" so it reads as the
    // range it is.
    versionOptions(p, res) {
        const selected = res.mode === "pinned" ? `pin:${p.pinnedVersionId}` : res.mode;
        const opt = (value, label) =>
            `<option value="${escapeHtml(value)}" ${value === selected ? "selected" : ""}>${escapeHtml(label)}</option>`;

        // When a mode is the active one its resolution already names the
        // exact build; otherwise fall back to describing the target.
        const declared =
            res.mode === "project" && res.status === "ready"
                ? `Project's version — ${res.label}`
                : `Project's version — ${declaredLabel(p) || "not recorded"}`;
        const fallback =
            res.mode === "default" && res.status === "ready"
                ? `Use default — ${res.label}`
                : this.defaultVersion
                  ? `Use default — ${installedLabel(this.defaultVersion)}`
                  : "Use default — none set";

        const builds = this.installedVersions
            .map((v) => opt(`pin:${v.id}`, installedLabel(v)))
            .join("");

        return `
            ${opt("project", declared)}
            ${opt("default", fallback)}
            ${builds ? `<optgroup label="Pin to a specific version">${builds}</optgroup>` : ""}`;
    },

    // renderNote renders the row's standing version note: what this project
    // needs and whether it can open. Distinct from the transient status
    // line, which reports what a click is currently doing.
    renderNote(res) {
        if (res.status === "missing") {
            return `<div class="project-row-note is-blocking">
                        <span>Needs Godot ${escapeHtml(res.label)} — not installed.</span>
                        <button class="btn btn-sm" data-action="install" type="button">Install it</button>
                    </div>`;
        }
        if (res.status === "unknown") {
            return `<div class="project-row-note is-blocking">
                        <span>${escapeHtml(res.warning || "No Godot version chosen yet.")}</span>
                    </div>`;
        }
        if (res.warning) {
            return `<div class="project-row-note is-warning">⚠ ${escapeHtml(res.warning)}</div>`;
        }
        return "";
    },

    renderRow(p) {
        const row = document.createElement("div");
        row.className = "project-row";
        row.dataset.id = p.id;

        const res = p.resolution || { mode: "project", status: "unknown", label: "", warning: "" };
        // The badge answers "which Godot does this open in", so it names the
        // resolved build by its full tag. Only when no build is settled on
        // does it fall back to the series the project records.
        const versionLabel = res.status === "ready" ? res.label : declaredLabel(p);
        const thumb = p.thumbnail
            ? `<div class="project-thumb"><img src="${p.thumbnail}" alt="" /></div>`
            : `<div class="project-thumb is-empty" aria-hidden="true">◇</div>`;

        row.innerHTML = `
            <div class="project-row-main">
                ${thumb}
                <div class="project-row-info">
                    <div class="project-row-name">
                        ${escapeHtml(p.name)}
                        ${p.usesCSharp ? '<span class="badge">C#</span>' : ""}
                        ${versionLabel ? `<span class="badge">Godot ${escapeHtml(versionLabel)}</span>` : ""}
                        ${p.missing ? '<span class="badge badge-danger">missing</span>' : ""}
                    </div>
                    <div class="muted project-row-path">${escapeHtml(p.path)}</div>
                    <input class="tags-input" data-action="tags" type="text" placeholder="tags, comma, separated" aria-label="Tags for ${escapeHtml(p.name)}" value="${escapeHtml((p.tags || []).join(", "))}" />
                </div>
            </div>
            <div class="project-row-actions">
                <select class="version-select" data-action="version" aria-label="Godot version for ${escapeHtml(p.name)}">
                    ${this.versionOptions(p, res)}
                </select>
                <button class="btn btn-sm btn-accent" data-action="open" type="button">Open</button>
                <button class="btn btn-sm" data-action="remove" type="button">Remove</button>
                <button class="star-btn ${p.favorite ? "is-favorite" : ""}" data-action="favorite" type="button" aria-pressed="${p.favorite}" aria-label="${p.favorite ? "Remove from favorites" : "Add to favorites"}">★</button>
            </div>
            ${this.renderNote(res)}
            <div class="project-row-status muted" data-status hidden></div>`;

        row.querySelector('[data-action="favorite"]').addEventListener("click", () => this.toggleFavorite(p));
        row.querySelector('[data-action="tags"]').addEventListener("change", (e) => this.saveTags(p, e.target.value));
        row.querySelector('[data-action="version"]').addEventListener("change", (e) => this.setVersion(p, e.target.value));
        row.querySelector('[data-action="open"]').addEventListener("click", () => this.openProject(p, row));
        row.querySelector('[data-action="remove"]').addEventListener("click", () => this.removeProject(p));
        row.querySelector('[data-action="install"]')?.addEventListener("click", () => this.installOnly(p, row));

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

    // setVersion maps the menu's value back to a mode: "pin:<id>" pins that
    // exact build, anything else is the mode itself.
    async setVersion(p, value) {
        const [mode, versionId] = value.startsWith("pin:")
            ? ["pinned", value.slice("pin:".length)]
            : [value, ""];
        try {
            await api.setProjectVersion(p.id, mode, versionId);
        } catch (err) {
            alert(`Failed to set version: ${err}`);
        }
        await this.refresh(); // the note below the row depends on the new choice
    },

    // installFor fetches the version a project needs. Returns whether it
    // succeeded, so the open flow can chain onto it. Deliberately doesn't
    // refresh -- that would re-render and discard the row the caller is
    // still reporting status into.
    //
    // Progress is reported in the row itself rather than pointing at the
    // Versions page: an install started from here is usually a step on the
    // way to opening the project, and sending the user to another page to
    // find out whether it's moving loses them the thing they were doing.
    // Same events, same bar, drawn where the click happened.
    async installFor(p, row) {
        const status = row.querySelector("[data-status]");
        const progress = this.showRowProgress(status, p);
        row.classList.add("is-installing");
        try {
            await api.installForProject(p.id);
            return true;
        } catch (err) {
            alert(`Failed to install: ${err}`);
            return false;
        } finally {
            progress.stop();
            row.classList.remove("is-installing");
            status.hidden = true;
        }
    },

    // showRowProgress draws a labelled bar into the row's status line and
    // drives it off the same install events the Versions page listens to.
    // Returns a stop() that unsubscribes -- these are per-install, so
    // leaving them attached would have every row a user ever installed from
    // still redrawing on the next download.
    showRowProgress(status, p) {
        const target = p.resolution?.label ? ` Godot ${p.resolution.label}` : "";
        status.hidden = false;
        status.innerHTML = `
            <div class="row-progress">
                <span data-progress-label>Starting download of${escapeHtml(target) || " the version this project needs"}…</span>
                <div class="progress-bar is-indeterminate" role="progressbar" aria-valuemin="0" aria-valuemax="100">
                    <div class="progress-bar-fill" data-progress-fill></div>
                </div>
            </div>`;

        const label = status.querySelector("[data-progress-label]");
        const fill = status.querySelector("[data-progress-fill]");
        const bar = status.querySelector(".progress-bar");

        const unsubscribe = [
            events.on("download_progress", (evt) => {
                const data = evt?.data;
                if (!data || !data.total) return;
                const pct = Math.min(100, Math.round((data.downloaded / data.total) * 100));
                bar.classList.remove("is-indeterminate"); // real numbers to show now
                fill.style.width = `${pct}%`;
                bar.setAttribute("aria-valuenow", String(pct));
                label.textContent = `Downloading${target} — ${pct}%`;
            }),
            events.on("checksum_verified", (evt) => {
                label.textContent = evt?.data?.verified
                    ? "Verified — extracting…"
                    : "Extracting… (checksum not published for this release)";
            }),
        ];

        return { stop: () => unsubscribe.forEach((off) => off()) };
    },

    // installOnly is the row's "Install it" button: fetch the version, then
    // refresh so the row stops saying it's missing.
    async installOnly(p, row) {
        await this.installFor(p, row);
        await this.refresh();
    },

    // openProject launches the project, but never quietly resolves a
    // problem on the user's behalf first: a missing version is offered as
    // a download they can decline, and an editor newer than the project
    // declares is confirmed before it gets the chance to rewrite the
    // project's files.
    async openProject(p, row) {
        const res = p.resolution || {};

        if (res.status === "unknown") {
            alert(`Can't open "${p.name}" yet.\n\n${res.warning || "No Godot version has been chosen for it."}\n\nPick one from the version menu on this row.`);
            return;
        }

        if (res.status === "missing") {
            const proceed = confirm(
                `"${p.name}" needs Godot ${res.label}, which isn't installed.\n\n` +
                `Download and install it now, then open the project?`
            );
            if (!proceed) return;
            if (!(await this.installFor(p, row))) {
                await this.refresh();
                return;
            }
            // The backend re-resolves on open, so the now-stale res isn't
            // consulted again -- and the version just installed is the one
            // the project asked for, so there's nothing left to confirm.
        } else if (res.willUpgrade) {
            const proceed = confirm(
                `${res.warning}\n\n` +
                `Open "${p.name}" in Godot ${res.label} anyway?`
            );
            if (!proceed) return;
        }

        const status = row.querySelector("[data-status]");
        status.hidden = false;
        status.textContent = `Opening in Godot ${res.label}…`;
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
