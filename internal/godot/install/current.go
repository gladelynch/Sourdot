package install

// Deliberately no symlink-based "current version" pointer -- see the plan's
// rationale. Every launch resolves a specific project's pinned
// InstalledVersion.BinaryPath directly via ProjectManager, so there's no
// OS-level pointer to switch and no admin-elevation problem class. An
// optional "default version for unpinned projects" is just a plain ID
// string in store.Settings.DefaultVersionID. Implemented in M1/M3.
