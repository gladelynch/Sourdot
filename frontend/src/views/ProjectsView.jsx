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
// Rendered as a plain list rather than a virtualized one -- correct at the
// scale a single developer's project list realistically reaches; revisit
// if that stops being true.
import { useState, useEffect, useMemo, useCallback } from "preact/hooks";
import { actions } from "../store.js";
import { useResource, useAsyncAction, useInstallProgress } from "../hooks.js";
import { ProgressBar, EmptyState, Badge } from "../components/common.jsx";
import { installedLabel, declaredLabel, failAlert } from "../utils.js";

export function ProjectsView() {
    const projects = useResource("projects");
    const installed = useResource("installed");
    const defaultID = useResource("defaultVersionID");

    const defaultVersion = useMemo(
        () => installed.find((v) => v.id === defaultID) || null,
        [installed, defaultID],
    );

    const [addProject, adding] = useAsyncAction("Failed to add project", () => actions.addProject());

    const sorted = useMemo(
        () =>
            [...projects].sort((a, b) => {
                if (a.favorite !== b.favorite) return a.favorite ? -1 : 1;
                return a.name.localeCompare(b.name);
            }),
        [projects],
    );

    return (
        <section id="view-projects" class="view" aria-labelledby="view-projects-heading">
            <header class="view-header">
                <h1 id="view-projects-heading">Projects</h1>
                <button class="btn btn-accent" type="button" disabled={adding} onClick={() => addProject()}>
                    Add project…
                </button>
            </header>

            {sorted.length === 0 ? (
                <EmptyState>
                    <p>No projects tracked yet.</p>
                    <p class="muted">Click "Add project…" to track one.</p>
                </EmptyState>
            ) : (
                <div class="project-list">
                    {sorted.map((p) => (
                        <ProjectRow
                            key={p.id}
                            project={p}
                            installed={installed}
                            defaultVersion={defaultVersion}
                        />
                    ))}
                </div>
            )}
        </section>
    );
}

function ProjectRow({ project: p, installed, defaultVersion }) {
    const res = p.resolution || { mode: "project", status: "unknown", label: "", warning: "" };

    // Transient status line: what a click is currently doing. Distinct
    // from the standing note below the row, which says what this project
    // needs regardless of any action.
    const [status, setStatus] = useState(null);
    const [installing, setInstalling] = useState(false);

    const tagsValue = (p.tags || []).join(", ");

    const progress = useInstallProgress(installing, res.label ? `Godot ${res.label}` : "");

    const [saveTags] = useAsyncAction("Failed to save tags", (tags) => actions.setTags(p.id, tags), [p.id]);
    const [toggleFavorite] = useAsyncAction(
        "Failed to update favorite",
        () => actions.setFavorite(p.id, !p.favorite),
        [p.id, p.favorite],
    );
    const [setVersion] = useAsyncAction(
        "Failed to set version",
        (value) => {
            // "pin:<id>" pins that exact build; anything else is the mode itself.
            const [mode, versionID] = value.startsWith("pin:")
                ? ["pinned", value.slice("pin:".length)]
                : [value, ""];
            return actions.setProjectVersion(p.id, mode, versionID);
        },
        [p.id],
    );
    const [removeProject] = useAsyncAction("Failed to remove project", () => actions.removeProject(p.id), [p.id]);

    // installFor fetches the version this project needs. Returns whether
    // it succeeded, so the open flow can chain onto it.
    //
    // Progress is reported in the row itself rather than pointing at the
    // Versions page: an install started from here is usually a step on the
    // way to opening the project, and sending the user to another page to
    // find out whether it's moving loses them the thing they were doing.
    // Same events, same bar, drawn where the click happened.
    const installFor = useCallback(async () => {
        setInstalling(true);
        try {
            await actions.installForProject(p.id);
            return true;
        } catch (err) {
            failAlert("Failed to install", err);
            return false;
        } finally {
            setInstalling(false);
        }
    }, [p.id]);

    // openProject launches the project, but never quietly resolves a
    // problem on the user's behalf first: a missing version is offered as
    // a download they can decline, and an editor newer than the project
    // declares is confirmed before it gets the chance to rewrite the
    // project's files.
    const openProject = useCallback(async () => {
        if (res.status === "unknown") {
            alert(
                `Can't open "${p.name}" yet.\n\n${res.warning || "No Godot version has been chosen for it."}\n\n` +
                    `Pick one from the version menu on this row.`,
            );
            return;
        }

        if (res.status === "missing") {
            const proceed = confirm(
                `"${p.name}" needs Godot ${res.label}, which isn't installed.\n\n` +
                    `Download and install it now, then open the project?`,
            );
            if (!proceed) return;
            if (!(await installFor())) return;
            // The backend re-resolves on open, so the now-stale res isn't
            // consulted again -- and the version just installed is the one
            // the project asked for, so there's nothing left to confirm.
        } else if (res.willUpgrade) {
            if (!confirm(`${res.warning}\n\nOpen "${p.name}" in Godot ${res.label} anyway?`)) return;
        }

        setStatus(`Opening in Godot ${res.label}…`);
        try {
            await actions.openProject(p.id);
        } catch (err) {
            failAlert("Failed to open project", err);
        }
        setStatus(null);
    }, [p.id, p.name, res.status, res.label, res.warning, res.willUpgrade, installFor]);

    // The badge answers "which Godot does this open in", so it names the
    // resolved build by its full tag. Only when no build is settled on
    // does it fall back to the series the project records.
    const versionLabel = res.status === "ready" ? res.label : declaredLabel(p);

    return (
        <div class={`project-row${installing ? " is-installing" : ""}`}>
            <div class="project-row-main">
                {p.thumbnail ? (
                    <div class="project-thumb">
                        <img src={p.thumbnail} alt="" />
                    </div>
                ) : (
                    <div class="project-thumb is-empty" aria-hidden="true">◇</div>
                )}
                <div class="project-row-info">
                    <div class="project-row-name">
                        {p.name}
                        {p.usesCSharp && <Badge>C#</Badge>}
                        {versionLabel && <Badge>Godot {versionLabel}</Badge>}
                        {p.missing && <Badge kind="badge-danger">missing</Badge>}
                    </div>
                    <div class="muted project-row-path">{p.path}</div>
                    <TagsInput name={p.name} value={tagsValue} onCommit={saveTags} />
                </div>
            </div>

            <div class="project-row-actions">
                <select
                    class="version-select"
                    aria-label={`Godot version for ${p.name}`}
                    value={res.mode === "pinned" ? `pin:${p.pinnedVersionId}` : res.mode}
                    onChange={(e) => setVersion(e.currentTarget.value)}
                >
                    <VersionOptions project={p} res={res} installed={installed} defaultVersion={defaultVersion} />
                </select>
                <button class="btn btn-sm btn-accent" type="button" onClick={openProject}>
                    Open
                </button>
                <button
                    class="btn btn-sm"
                    type="button"
                    onClick={() => {
                        if (confirm(`Stop tracking "${p.name}"? This won't delete any files.`)) removeProject();
                    }}
                >
                    Remove
                </button>
                <button
                    class={`star-btn${p.favorite ? " is-favorite" : ""}`}
                    type="button"
                    aria-pressed={p.favorite}
                    aria-label={p.favorite ? "Remove from favorites" : "Add to favorites"}
                    onClick={() => toggleFavorite()}
                >
                    ★
                </button>
            </div>

            <RowNote res={res} onInstall={installFor} />

            {(progress || status) && (
                <div class="project-row-status muted">
                    {progress ? (
                        <div class="row-progress">
                            <span>{progress.label}</span>
                            <ProgressBar percent={progress.percent} indeterminate={progress.indeterminate} />
                        </div>
                    ) : (
                        status
                    )}
                </div>
            )}
        </div>
    );
}

// TagsInput keeps what the user is typing in local state and only tells the
// store about it on commit (blur or Enter).
//
// Binding it straight to the store value would mean a round-trip per
// keystroke; rendering it uncontrolled would mean never picking up a change
// made elsewhere. The effect splits the difference: the draft is adopted
// from the store only when the stored value actually changes, so a refresh
// triggered by something else on the page -- an install finishing, a
// favourite toggling -- re-renders this row without eating a half-typed
// tag. That's the kind of thing the old innerHTML rebuild destroyed every
// time.
function TagsInput({ name, value, onCommit }) {
    const [draft, setDraft] = useState(value);
    useEffect(() => setDraft(value), [value]);

    const commit = () => {
        const tags = draft.split(",").map((t) => t.trim()).filter(Boolean);
        if (tags.join(", ") !== value) onCommit(tags);
    };

    return (
        <input
            class="tags-input"
            type="text"
            placeholder="tags, comma, separated"
            aria-label={`Tags for ${name}`}
            value={draft}
            onInput={(e) => setDraft(e.currentTarget.value)}
            onBlur={commit}
            onKeyDown={(e) => {
                if (e.key === "Enter") e.currentTarget.blur();
            }}
        />
    );
}

// RowNote renders the row's standing version note: what this project needs
// and whether it can open.
function RowNote({ res, onInstall }) {
    if (res.status === "missing") {
        return (
            <div class="project-row-note is-blocking">
                <span>Needs Godot {res.label} — not installed.</span>
                <button class="btn btn-sm" type="button" onClick={() => onInstall()}>
                    Install it
                </button>
            </div>
        );
    }
    if (res.status === "unknown") {
        return (
            <div class="project-row-note is-blocking">
                <span>{res.warning || "No Godot version chosen yet."}</span>
            </div>
        );
    }
    if (res.warning) {
        return <div class="project-row-note is-warning">⚠ {res.warning}</div>;
    }
    return null;
}

// VersionOptions builds the version menu: the project's own version first
// (and selected, for anything not explicitly changed), then the app-wide
// default, then every installed build. The first two spell out the build
// they currently mean, so "Project's version" isn't a mystery the user has
// to click Open to resolve.
//
// Every version here is named by its full release tag. A bare "4.8" is
// never shown: it's four different builds (4.8-dev3, 4.8-beta1, 4.8-rc1,
// 4.8-stable) wearing one name. Where only a series is known --
// project.godot records no build label at all, so "4.8" means whichever
// 4.8 is current -- it's written "4.8.x" so it reads as the range it is.
//
// `installed` arrives from the store, so installing or uninstalling a
// version anywhere in the app rebuilds this menu. It used to be a copy
// this view fetched once, which is why it went stale.
function VersionOptions({ project: p, res, installed, defaultVersion }) {
    // When a mode is the active one its resolution already names the exact
    // build; otherwise fall back to describing the target.
    const declared =
        res.mode === "project" && res.status === "ready"
            ? `Project's version — ${res.label}`
            : `Project's version — ${declaredLabel(p) || "not recorded"}`;

    const fallback =
        res.mode === "default" && res.status === "ready"
            ? `Use default — ${res.label}`
            : defaultVersion
              ? `Use default — ${installedLabel(defaultVersion)}`
              : "Use default — none set";

    return (
        <>
            <option value="project">{declared}</option>
            <option value="default">{fallback}</option>
            {installed.length > 0 && (
                <optgroup label="Pin to a specific version">
                    {installed.map((v) => (
                        <option key={v.id} value={`pin:${v.id}`}>
                            {installedLabel(v)}
                        </option>
                    ))}
                </optgroup>
            )}
        </>
    );
}
