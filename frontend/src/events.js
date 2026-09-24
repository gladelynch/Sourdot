// Wraps Wails runtime events (window.runtime.EventsOn) into a tiny pub/sub
// keyed by topic, so individual components can subscribe reactively
// instead of each registering its own EventsOn listener.
//
// This carries the transient, high-frequency side of the backend's output
// -- download percentages, checksum results -- which is deliberately *not*
// in the store: it belongs to one in-flight operation, not to the app's
// persistent state, and pushing every progress tick through a store
// refresh would re-render the world sixty times a download. What the store
// does get is the completion: see bindRuntimeEvents in main.jsx.
const subscribers = new Map(); // topic -> Set<fn>

export const events = {
    on(topic, fn) {
        if (!subscribers.has(topic)) subscribers.set(topic, new Set());
        subscribers.get(topic).add(fn);
        return () => subscribers.get(topic)?.delete(fn);
    },

    dispatch(topic, payload) {
        subscribers.get(topic)?.forEach((fn) => fn(payload));
    },

    // core.Event{Type, ID, Data} is emitted with the Wails event name equal
    // to its own Type, so each binding is a 1:1 passthrough into the
    // internal topic of the same name.
    bindRuntimeEvent(wailsEventName, topic) {
        if (!window.runtime?.EventsOn) return;
        window.runtime.EventsOn(wailsEventName, (payload) => events.dispatch(topic, payload));
    },
};
