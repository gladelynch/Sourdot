// Settings view: default version for unpinned projects, an optional GitHub
// PAT (raises the 60/hr unauthenticated API rate limit), the data
// directory (with a reveal-in-file-manager action), setup for the `godot`
// command-line launcher, and basic backend status/about info.
import { useState } from "preact/hooks";
import { api } from "../api.js";
import { actions } from "../store.js";
import { useResource, useAsyncAction } from "../hooks.js";
import { installedLabel, failAlert } from "../utils.js";

const PROJECT_REPO_URL = "https://github.com/gladelynch/Sourdot";

export function SettingsView() {
    // All four come from the store, so the default-version picker below
    // rebuilds whenever a version is installed or removed on the Versions
    // page. It used to read the installed list into a local at init and
    // never look again, which left it offering builds that were gone.
    const status = useResource("status");
    const logPath = useResource("logPath");
    const installed = useResource("installed");
    const defaultID = useResource("defaultVersionID");
    const hasToken = useResource("hasToken");
    const shimInfo = useResource("shimInfo");

    const [token, setToken] = useState("");

    const [setDefaultVersion] = useAsyncAction("Failed to set default version", (id) =>
        actions.setDefaultVersion(id),
    );
    const [saveToken, savingToken] = useAsyncAction("Failed to save token", async (value) => {
        await actions.setGitHubToken(value);
        setToken("");
    });

    return (
        <section id="view-settings" class="view" aria-labelledby="view-settings-heading">
            <header class="view-header">
                <h1 id="view-settings-heading">Settings</h1>
            </header>

            <div class="settings-panel">
                <div class="settings-field">
                    <label for="default-version-select">Default version for unpinned projects</label>
                    <select
                        id="default-version-select"
                        value={defaultID}
                        onChange={(e) => setDefaultVersion(e.currentTarget.value)}
                    >
                        <option value="">(none)</option>
                        {installed.map((v) => (
                            <option key={v.id} value={v.id}>
                                {installedLabel(v)}
                            </option>
                        ))}
                    </select>
                </div>

                <div class="settings-field">
                    <label for="github-token-input">GitHub personal access token (optional)</label>
                    <div class="settings-field-row">
                        <input
                            id="github-token-input"
                            type="password"
                            autocomplete="off"
                            value={token}
                            onInput={(e) => setToken(e.currentTarget.value)}
                            placeholder={
                                hasToken
                                    ? "Token set — enter a new one to replace it"
                                    : "Raises the 60/hr unauthenticated rate limit"
                            }
                        />
                        <button
                            class="btn btn-sm"
                            type="button"
                            disabled={savingToken}
                            onClick={() => saveToken(token.trim())}
                        >
                            Save
                        </button>
                    </div>
                </div>

                <div class="settings-field">
                    <span class="settings-field-label" id="data-dir-label">Data directory</span>
                    <div class="settings-field-row">
                        <code class="muted" aria-labelledby="data-dir-label">{status?.dataDir || "—"}</code>
                        <button class="btn btn-sm" type="button" onClick={() => reveal()}>
                            Reveal
                        </button>
                    </div>
                </div>

                {/* The log lives in the data directory, so the Reveal button
                    above already exposes it; this row just names the file to
                    attach to a bug report. */}
                <div class="settings-field">
                    <span class="settings-field-label" id="log-path-label">Log file</span>
                    <div class="settings-field-row">
                        <code class="muted" aria-labelledby="log-path-label">
                            {logPath || "(not writing to a file)"}
                        </code>
                    </div>
                </div>
            </div>

            {shimInfo && <LauncherPanel info={shimInfo} />}

            <div class="settings-panel">
                <dl class="about-list">
                    <dt>Backend status</dt>
                    <dd>{status ? `ok (schema v${status.schemaVersion})` : "unreachable"}</dd>
                    <dt>App version</dt>
                    <dd>{status?.appVersion || "dev"}</dd>
                    <dt>Source</dt>
                    <dd>
                        <button class="btn btn-sm btn-link" type="button" onClick={() => openRepo()}>
                            github.com/gladelynch/Sourdot ↗
                        </button>
                    </dd>
                </dl>
            </div>
        </section>
    );
}

// The `godot` launcher: Sourdot keeps it in its bin directory, and once that
// is on PATH, `godot` runs whichever build the surrounding project uses --
// so C# test runners and scripts get the right engine with no hard-coded
// path. Sourdot never edits shell profiles itself; it shows the lines.
function LauncherPanel({ info }) {
    const { setup } = info;
    const where = setup.profile
        ? `Add these lines to ${setup.profile}, then open a new terminal:`
        : "Run these once in PowerShell, then open a new terminal:";

    return (
        <div class="settings-panel">
            <div class="settings-field">
                <span class="settings-field-label">Command-line launcher</span>
                <p class="muted settings-help">
                    Put Sourdot's <code>godot</code> on your PATH and running <code>godot</code> inside a
                    tracked project uses that project's version; outside any project it uses the default
                    version. <code>GODOT_BIN</code> points test runners such as GdUnit4 at the same launcher.
                    {info.onPath && " Sourdot's own PATH already includes it."}
                </p>
                {info.error && <p class="settings-error">Couldn't update the launcher: {info.error}</p>}
            </div>

            <div class="settings-field">
                <span class="settings-field-label">{where}</span>
                {setup.lines.map((line) => (
                    <div class="settings-field-row" key={line}>
                        <code>{line}</code>
                        <button class="btn btn-sm" type="button" onClick={() => copy(line)}>
                            Copy
                        </button>
                    </div>
                ))}
            </div>
        </div>
    );
}

async function copy(text) {
    try {
        await api.copyText(text);
    } catch (err) {
        failAlert("Couldn't copy to the clipboard", err);
    }
}

// Handed to the OS browser rather than navigated to in-app; the Go side
// allowlists the host before opening anything.
async function openRepo() {
    try {
        await api.openURL(PROJECT_REPO_URL);
    } catch (err) {
        failAlert(`Couldn't open ${PROJECT_REPO_URL}`, err);
    }
}

async function reveal() {
    try {
        await api.openDataDir();
    } catch (err) {
        failAlert("Failed to open data directory", err);
    }
}
