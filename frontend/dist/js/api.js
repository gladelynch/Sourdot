// Thin wrapper over Wails' runtime-injected window.go.main.App.* bindings.
// No codegen/build step involved — Wails injects these functions itself at
// runtime, so this file just gives the rest of the frontend friendlier names
// and a single place to adapt if the Go-side method signatures change.
const api = {
    ping() {
        return window.go.main.App.Ping();
    },
};
