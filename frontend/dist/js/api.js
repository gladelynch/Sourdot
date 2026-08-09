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
    setPinnedVersion(id, versionId) {
        return window.go.main.App.SetPinnedVersion(id, versionId);
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
};
