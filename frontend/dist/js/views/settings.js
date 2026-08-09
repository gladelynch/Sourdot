// Settings view — full settings ship in M5. For M0 this just smoke-tests
// the Go<->JS binding by calling App.Ping() and rendering the result.
const settingsView = {
    async init() {
        const statusEl = document.getElementById("backend-status");
        const dataDirEl = document.getElementById("data-dir");
        try {
            const result = await api.ping();
            statusEl.textContent = `ok (schema v${result.schemaVersion})`;
            dataDirEl.textContent = result.dataDir;
        } catch (err) {
            statusEl.textContent = "unreachable";
            console.error("ping failed:", err);
        }
    },
};
