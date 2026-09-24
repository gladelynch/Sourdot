// The bridge between the store and Preact's render cycle.
import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { store } from "./store.js";
import { events } from "./events.js";
import { failAlert } from "./utils.js";

// useResource subscribes a component to one store resource. Every consumer
// of a resource re-renders when it changes, whichever view triggered the
// change -- which is the whole point of the exercise: installing a version
// on the Versions page now updates the Projects page's version menu
// because both are reading the same `installed`, not two copies of it.
export function useResource(key) {
    const [value, setValue] = useState(() => store.get(key));
    useEffect(() => {
        setValue(store.get(key)); // catch a load that landed between render and effect
        return store.subscribe(key, setValue);
    }, [key]);
    return value;
}

// useAsyncAction wraps a mutation in the two things every call site here
// wanted anyway: a pending flag to disable the control with, and a single
// place errors get reported from. `label` becomes the alert's prefix.
export function useAsyncAction(label, fn, deps = []) {
    const [pending, setPending] = useState(false);
    const alive = useRef(true);
    useEffect(() => () => { alive.current = false; }, []);

    const run = useCallback(async (...args) => {
        setPending(true);
        try {
            return await fn(...args);
        } catch (err) {
            failAlert(label, err);
            return undefined;
        } finally {
            // The Projects list unmounts rows on refresh, so a mutation
            // that triggers one can outlive the component that started it.
            if (alive.current) setPending(false);
        }
    }, deps); // eslint-disable-line react-hooks/exhaustive-deps

    return [run, pending];
}

// useRuntimeEvent subscribes to a backend event topic for the lifetime of
// the component. Returning the unsubscribe from the effect is what stops
// the old bug where every row a user had ever installed from stayed
// attached and redrew on the next unrelated download.
export function useRuntimeEvent(topic, fn, deps = []) {
    useEffect(() => events.on(topic, fn), deps); // eslint-disable-line react-hooks/exhaustive-deps
}

// useInstallProgress tracks one install's progress events and renders them
// down to {label, percent, indeterminate}. `active` gates it so a caller
// only listens while its own install is running -- these events are
// global, and an unfiltered listener would have every row on the page
// narrating somebody else's download.
export function useInstallProgress(active, target = "", initialLabel = "") {
    const [state, setState] = useState(null);

    useEffect(() => {
        if (!active) {
            setState(null);
            return undefined;
        }
        setState({
            label: initialLabel || `Starting download of${target ? ` ${target}` : " the version this project needs"}…`,
            percent: 0,
            indeterminate: true,
        });

        const off = [
            events.on("download_progress", (evt) => {
                const data = evt?.data;
                if (!data || !data.total) return;
                const percent = Math.min(100, Math.round((data.downloaded / data.total) * 100));
                setState({
                    label: `Downloading${target ? ` ${target}` : ""} — ${percent}%`,
                    percent,
                    indeterminate: false, // real numbers to show now
                });
            }),
            events.on("checksum_verified", (evt) => {
                setState((prev) => ({
                    ...(prev || { percent: 100, indeterminate: false }),
                    label: evt?.data?.verified
                        ? "Verified — extracting…"
                        : "Extracting… (checksum not published for this release)",
                }));
            }),
        ];
        return () => off.forEach((fn) => fn());
    }, [active, target, initialLabel]);

    return state;
}
