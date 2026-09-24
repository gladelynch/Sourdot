// App shell: the sidebar and the three top-level views.
//
// All three views stay mounted and the inactive ones are hidden with the
// `hidden` attribute (.view[hidden] in main.css), rather than being
// unmounted on every tab switch. That keeps a view's local UI state --
// the Versions page's search box, channel filter and which series are
// expanded -- alive across a trip to Projects and back, which is what the
// old hidden-div markup did too. Server state doesn't depend on it either
// way: that lives in the store.
import { useState } from "preact/hooks";
import { VersionsView } from "../views/VersionsView.jsx";
import { ProjectsView } from "../views/ProjectsView.jsx";
import { SettingsView } from "../views/SettingsView.jsx";

const NAV = [
    ["versions", "Versions"],
    ["projects", "Projects"],
    ["settings", "Settings"],
];

export function App() {
    const [view, setView] = useState("versions");

    const navButton = (id, label) => (
        <button
            key={id}
            class={`nav-item${view === id ? " is-active" : ""}`}
            data-view={id}
            type="button"
            onClick={() => setView(id)}
        >
            {label}
        </button>
    );

    return (
        <div class="app">
            <nav class="sidebar" aria-label="Primary">
                <div class="brand">
                    <div class="brand-name">Sourdot</div>
                    <div class="brand-tagline">Godot Version Manager</div>
                </div>
                {navButton("versions", "Versions")}
                {navButton("projects", "Projects")}
                <div class="nav-spacer" />
                {navButton("settings", "Settings")}
            </nav>

            <main class="content">
                <div hidden={view !== "versions"}><VersionsView /></div>
                <div hidden={view !== "projects"}><ProjectsView /></div>
                <div hidden={view !== "settings"}><SettingsView /></div>
            </main>
        </div>
    );
}
