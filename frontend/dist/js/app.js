// App shell: tab switching between the three top-level views, wires up the
// Wails runtime event bridge once, then hands off to each view's init().
document.addEventListener("DOMContentLoaded", () => {
    const navItems = document.querySelectorAll(".nav-item[data-view]");
    const viewEls = document.querySelectorAll(".view");

    navItems.forEach((item) => {
        item.addEventListener("click", () => {
            const target = item.dataset.view;
            navItems.forEach((n) => n.classList.toggle("is-active", n === item));
            viewEls.forEach((v) => {
                v.hidden = v.id !== `view-${target}`;
            });
        });
    });

    // core.Event{Type, ID, Data} is emitted with the Wails event name equal
    // to its own Type, so each of these is a 1:1 passthrough into our
    // internal pub/sub topic of the same name (see js/events.js).
    ["download_progress", "checksum_verified", "install_complete"].forEach((name) => {
        events.bindRuntimeEvent(name, name);
    });

    versionsView.init();
    projectsView.init();
    settingsView.init();
});
