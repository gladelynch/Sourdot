// Presentational pieces shared by more than one view.

// ProgressBar renders the download/extract bar. `indeterminate` is the
// state before the first byte count arrives -- the backend doesn't know a
// release's size until the response headers land, and a bar sitting at 0%
// reads as "stuck" rather than "starting".
export function ProgressBar({ percent = 0, indeterminate = false, labelledBy }) {
    return (
        <div
            class={`progress-bar${indeterminate ? " is-indeterminate" : ""}`}
            role="progressbar"
            aria-valuemin="0"
            aria-valuemax="100"
            aria-valuenow={indeterminate ? undefined : percent}
            aria-labelledby={labelledBy}
        >
            <div class="progress-bar-fill" style={{ width: `${percent}%` }} />
        </div>
    );
}

export function EmptyState({ children }) {
    return <div class="empty-state">{children}</div>;
}

export function Badge({ kind, children }) {
    return <span class={kind ? `badge ${kind}` : "badge"}>{children}</span>;
}
