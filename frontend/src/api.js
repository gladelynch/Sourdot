// Thin wrapper over Wails' runtime-injected window.go.main.App.* bindings.
//
// Wails injects these functions itself at runtime, so nothing here is
// generated -- this file just gives the rest of the frontend friendlier
// names and a single place to adapt if the Go-side method signatures
// change. That indirection is the reason the SetVersionMode rename below
// only had to happen once.
export const api = {
    ping() {
        return window.go.main.App.Ping();
    },

    listAvailableVersions() {
        return window.go.main.App.ListAvailableVersions();
    },
    refreshAvailableVersions() {
        return window.go.main.App.RefreshAvailableVersions();
    },
    listInstalledVersions() {
        return window.go.main.App.ListInstalledVersions();
    },
    installVersion(tagName, isMono) {
        return window.go.main.App.InstallVersion(tagName, isMono);
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

    pickAndAddProject() {
        return window.go.main.App.PickAndAddProject();
    },
    listProjects() {
        return window.go.main.App.ListProjects();
    },
    removeProject(id) {
        return window.go.main.App.RemoveProject(id);
    },
    setFavorite(id, favorite) {
        return window.go.main.App.SetFavorite(id, favorite);
    },
    setTags(id, tags) {
        return window.go.main.App.SetTags(id, tags);
    },
    // The Go binding is SetVersionMode; this called SetProjectVersion, which
    // has never existed on the App struct, so every version change from the
    // Projects page threw "not a function" before it reached the backend.
    setProjectVersion(id, mode, versionId) {
        return window.go.main.App.SetVersionMode(id, mode, versionId || "");
    },
    installForProject(id) {
        return window.go.main.App.InstallForProject(id);
    },
    openProject(id) {
        return window.go.main.App.OpenProject(id);
    },

    hasGitHubToken() {
        return window.go.main.App.HasGitHubToken();
    },
    setGitHubToken(token) {
        return window.go.main.App.SetGitHubToken(token);
    },
    openDataDir() {
        return window.go.main.App.OpenDataDir();
    },
    getLogPath() {
        return window.go.main.App.GetLogPath();
    },
    openURL(url) {
        return window.go.main.App.OpenURL(url);
    },
};
