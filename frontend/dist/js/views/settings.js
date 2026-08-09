// Settings view: default version for unpinned projects, an optional
// GitHub PAT (raises the 60/hr unauthenticated API rate limit), the data
// directory (with a reveal-in-file-manager action), and basic backend
// status/about info.
const PROJECT_REPO_URL = "https://github.com/gladelynch/Sourdot";

const settingsView = {
    async init() {
        document.getElementById("github-token-save").addEventListener("click", () => this.saveToken());
        document.getElementById("reveal-data-dir-btn").addEventListener("click", () => this.revealDataDir());
        document.getElementById("project-repo-btn").addEventListener("click", () => this.openRepo());

        await Promise.all([this.loadStatus(), this.loadDefaultVersion(), this.loadTokenStatus()]);
    },

    async loadStatus() {
        const statusEl = document.getElementById("backend-status");
        try {
            const result = await api.ping();
            statusEl.textContent = `ok (schema v${result.schemaVersion})`;
            document.getElementById("data-dir-value").textContent = result.dataDir;
            document.getElementById("app-version").textContent = result.appVersion || "dev";
        } catch (err) {
            statusEl.textContent = "unreachable";
            console.error("ping failed:", err);
        }
    },

    async loadDefaultVersion() {
        const select = document.getElementById("default-version-select");
        let installed = [];
        let defaultID = "";
        try {
            [installed, defaultID] = await Promise.all([
                api.listInstalledVersions().then((v) => v || []),
                api.getDefaultVersion(),
            ]);
        } catch (err) {
            console.error("failed to load default version:", err);
            return;
        }

        const options = installed
            .map((v) => {
                const selected = v.id === defaultID ? "selected" : "";
                return `<option value="${escapeHtml(v.id)}" ${selected}>${escapeHtml(installedLabel(v))}</option>`;
            })
            .join("");
        select.innerHTML = `<option value="">(none)</option>${options}`;

        select.addEventListener("change", async () => {
            try {
                await api.setDefaultVersion(select.value);
            } catch (err) {
                alert(`Failed to set default version: ${err}`);
            }
        });
    },

    async loadTokenStatus() {
        const input = document.getElementById("github-token-input");
        try {
            const has = await api.hasGitHubToken();
            input.placeholder = has ? "Token set — enter a new one to replace it" : "Raises the 60/hr unauthenticated rate limit";
        } catch (err) {
            console.error("failed to check token status:", err);
        }
    },

    async saveToken() {
        const input = document.getElementById("github-token-input");
        try {
            await api.setGitHubToken(input.value.trim());
            input.value = "";
            await this.loadTokenStatus();
        } catch (err) {
            alert(`Failed to save token: ${err}`);
        }
    },

    // Handed to the OS browser rather than navigated to in-app; the Go side
    // allowlists the host before opening anything.
    async openRepo() {
        try {
            await api.openURL(PROJECT_REPO_URL);
        } catch (err) {
            alert(`Couldn't open ${PROJECT_REPO_URL}:\n${err}`);
        }
    },

    async revealDataDir() {
        try {
            await api.openDataDir();
        } catch (err) {
            alert(`Failed to open data directory: ${err}`);
        }
    },
};
