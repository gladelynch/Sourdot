// Entry point: install the error handlers, bridge the backend's runtime
// events, and mount the app.
import { render } from "preact";
import { App } from "./components/App.jsx";
import { events } from "./events.js";
import { store } from "./store.js";
import { installGlobalErrorHandlers } from "./utils.js";

installGlobalErrorHandlers();

// The three topics the backend emits on. download_progress and
// checksum_verified are consumed directly by whichever component is
// showing a progress bar; install_complete additionally invalidates the
// store below.
for (const name of ["download_progress", "checksum_verified", "install_complete"]) {
    events.bindRuntimeEvent(name, name);
}

// An install can finish without any frontend action having started it:
// OpenProject auto-installs the version a project resolves to, entirely
// inside the Go side. Hanging the refresh off the event as well as off the
// actions means the app converges on the truth no matter who kicked the
// install off -- including the backend, by itself.
events.on("install_complete", () => {
    void store.refresh("installed", "projects", "defaultVersionID");
});

render(<App />, document.getElementById("root"));
