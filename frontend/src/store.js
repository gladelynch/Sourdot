// The single owner of every piece of server-backed state.
//
// Before this existed each view fetched its own copy of the same data and
// kept it in a private field: the Versions page had `state.installed`, the
// Projects page had `installedVersions`, and Settings read the list into a
// local that was never looked at again. Installing a version refreshed
// exactly one of those three, so the Projects page's version menu and the
// Settings default picker went on showing a list that no longer matched
// the disk. That is a data-ownership bug, not a rendering one -- moving to
// Preact on its own would have reproduced it exactly, with three
// components each fetching in their own effect.
//
// So: one cache, keyed by resource. Views never fetch; they subscribe.
// Mutations never return data for a caller to stash; they name the
// resources they invalidated and the store refetches those and notifies
// everyone holding them. Adding a new mutation means answering one
// question -- what does this make stale? -- and cross-view staleness stops
// being something you can forget to handle.
import { api } from "./api.js";
import { logError } from "./utils.js";

// Every resource the frontend reads, and how to load one. The keys are the
// vocabulary the rest of the app uses to talk about staleness.
const loaders = {
    installed: () => api.listInstalledVersions().then((v) => v || []),
    projects: () => api.listProjects().then((p) => p || []),
    defaultVersionID: () => api.getDefaultVersion().then((id) => id || ""),
    catalog: () => api.listAvailableVersions().then((g) => g || []),
    status: () => api.ping(),
    logPath: () => api.getLogPath(),
    hasToken: () => api.hasGitHubToken(),
};

const empty = {
    installed: [],
    projects: [],
    defaultVersionID: "",
    catalog: [],
    status: null,
    logPath: "",
    hasToken: false,
};

const cache = { ...empty };
const loaded = new Set();           // keys fetched at least once
const inflight = new Map();         // key -> Promise, so N subscribers share one call
const subscribers = new Map();      // key -> Set<fn>

function notify(key) {
    subscribers.get(key)?.forEach((fn) => fn(cache[key]));
}

export const store = {
    get(key) {
        return cache[key];
    },

    isLoaded(key) {
        return loaded.has(key);
    },

    // subscribe registers fn for a resource and returns an unsubscribe.
    // Mounting a subscriber triggers the first load; later mounts reuse the
    // cached value, so switching tabs is instant and doesn't re-hit the
    // backend.
    subscribe(key, fn) {
        if (!subscribers.has(key)) subscribers.set(key, new Set());
        subscribers.get(key).add(fn);
        if (!loaded.has(key)) void store.load(key);
        return () => subscribers.get(key)?.delete(fn);
    },

    // load fetches a resource unless it is already cached. Concurrent calls
    // for the same key share one request.
    load(key) {
        if (loaded.has(key)) return Promise.resolve(cache[key]);
        return store.refresh(key);
    },

    // refresh refetches the named resources unconditionally and notifies
    // their subscribers. This is what a mutation calls to declare what it
    // made stale.
    async refresh(...keys) {
        await Promise.all(keys.map(refreshOne));
    },
};

async function refreshOne(key) {
    const pending = inflight.get(key);
    if (pending) return pending; // a refresh is already on the wire; ride it

    const p = (async () => {
        try {
            cache[key] = await loaders[key]();
        } catch (err) {
            logError(`failed to load ${key}`, err);
            // Keep the last good value rather than blanking the UI on a
            // transient failure -- except on the very first load, where
            // there is no last good value and the empty shape is what the
            // components are written against.
            if (!loaded.has(key)) cache[key] = empty[key];
        } finally {
            loaded.add(key);
            inflight.delete(key);
        }
        notify(key);
        return cache[key];
    })();

    inflight.set(key, p);
    return p;
}

// --- Mutations --------------------------------------------------------
//
// Each one performs the call and then names what it invalidated. The
// comments say *why* a resource is in the list where it isn't obvious --
// most of the cross-view bugs lived in exactly those non-obvious entries.

export const actions = {
    async installVersion(tagName, isMono) {
        await api.installVersion(tagName, isMono);
        // "projects" because a resolution that read "needs 4.8, not
        // installed" may now be satisfied by this build.
        await store.refresh("installed", "projects");
    },

    async removeVersion(id) {
        await api.removeVersion(id);
        // "defaultVersionID" because the backend clears the default when
        // the build it pointed at is the one being removed.
        await store.refresh("installed", "projects", "defaultVersionID");
    },

    async setDefaultVersion(id) {
        await api.setDefaultVersion(id);
        // "projects" because every project in "default" mode resolves
        // through this.
        await store.refresh("defaultVersionID", "projects");
    },

    async refreshCatalog() {
        cache.catalog = (await api.refreshAvailableVersions()) || [];
        loaded.add("catalog");
        notify("catalog");
    },

    async addProject() {
        const proj = await api.pickAndAddProject();
        if (!proj) return null; // user cancelled the folder picker
        await store.refresh("projects");
        return proj;
    },

    async removeProject(id) {
        await api.removeProject(id);
        await store.refresh("projects");
    },

    async setFavorite(id, favorite) {
        await api.setFavorite(id, favorite);
        await store.refresh("projects");
    },

    async setTags(id, tags) {
        await api.setTags(id, tags);
        await store.refresh("projects");
    },

    async setProjectVersion(id, mode, versionID) {
        await api.setProjectVersion(id, mode, versionID);
        await store.refresh("projects");
    },

    async installForProject(id) {
        await api.installForProject(id);
        await store.refresh("installed", "projects");
    },

    async openProject(id) {
        await api.openProject(id);
        await store.refresh("projects"); // pick up the updated "last opened" state
    },

    async setGitHubToken(token) {
        await api.setGitHubToken(token);
        await store.refresh("hasToken");
    },
};
