// Wraps Wails runtime events (window.runtime.EventsOn/Off) into a tiny
// pub/sub keyed by id, so individual list rows/components can subscribe
// reactively instead of each registering its own EventsOn listener.
//
// Not wired to anything yet — scaffolded now so the pattern is established
// ahead of M2's download-progress events and M4's per-project install state.
const events = (() => {
    const subscribers = new Map(); // topic -> Set<fn>

    function on(topic, fn) {
        if (!subscribers.has(topic)) subscribers.set(topic, new Set());
        subscribers.get(topic).add(fn);
        return () => subscribers.get(topic)?.delete(fn);
    }

    function dispatch(topic, payload) {
        subscribers.get(topic)?.forEach((fn) => fn(payload));
    }

    function bindRuntimeEvent(wailsEventName, topic) {
        if (!window.runtime?.EventsOn) return;
        window.runtime.EventsOn(wailsEventName, (payload) => dispatch(topic, payload));
    }

    return { on, dispatch, bindRuntimeEvent };
})();
