// App shell: tab switching between the three top-level views, then hands
// off to each view's own init().
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

    versionsView.init();
    projectsView.init();
    settingsView.init();
});
