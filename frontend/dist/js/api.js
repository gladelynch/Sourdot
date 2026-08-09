// Thin wrapper over Wails' runtime-injected window.go.main.App.* bindings.
// No codegen/build step involved — Wails injects these functions itself at
// runtime, so this file just gives the rest of the frontend friendlier names
// and a single place to adapt if the Go-side method signatures change.
const api = {
    ping() {
        return window.go.main.App.Ping();
    },

    listAvailableVersions() {
        return window.go.main.App.ListAvailableVersions();
    },
    listInstalledVersions() {
        return window.go.main.App.ListInstalledVersions();
    },
    installVersion(release, isMono) {
        return window.go.main.App.InstallVersion(release, isMono);
    },
    removeVersion(id) {
        return window.go.main.App.RemoveVersion(id);
    },
    getDefaultVersion() {
        return window.go.main.App.GetDefaultVersion();
    },
    setDefaultVersion(id) {
        return window.go.main.App.SetDefaultVersion(id);
    },
};
