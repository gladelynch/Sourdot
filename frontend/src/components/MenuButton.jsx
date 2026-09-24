// A button that opens a small dropdown menu of actions.
//
// The menu is positioned `fixed` from the button's rect rather than
// absolutely inside the row, because .series-group sets overflow:hidden to
// clip its own rounded corners -- an absolutely positioned menu on the last
// row of a series would be cut off at the group's bottom edge. The flip
// side of fixed positioning is that the coordinates are only valid until
// something scrolls, so a scroll closes the menu rather than chasing it.
import { useState, useRef, useEffect, useCallback } from "preact/hooks";

export function MenuButton({ label, items, disabled, buttonClass = "btn btn-sm btn-accent", menuLabel }) {
    const [open, setOpen] = useState(false);
    const [pos, setPos] = useState({ top: 0, right: 0 });
    const btnRef = useRef(null);
    const menuRef = useRef(null);

    const close = useCallback((refocus) => {
        setOpen(false);
        if (refocus) btnRef.current?.focus();
    }, []);

    useEffect(() => {
        if (!open) return undefined;

        const onPointerDown = (e) => {
            if (!btnRef.current?.contains(e.target) && !menuRef.current?.contains(e.target)) close(false);
        };
        const onKeyDown = (e) => {
            if (e.key === "Escape") close(true);
        };
        const onScrollOrResize = () => close(false);

        document.addEventListener("mousedown", onPointerDown);
        document.addEventListener("keydown", onKeyDown);
        // Capture, so scrolling .content (the real scroll container) counts
        // and not just the window.
        window.addEventListener("scroll", onScrollOrResize, true);
        window.addEventListener("resize", onScrollOrResize);

        // Move focus into the menu so keyboard users aren't stranded on the
        // button with an open menu they can't reach.
        menuRef.current?.querySelector("button:not([disabled])")?.focus();

        return () => {
            document.removeEventListener("mousedown", onPointerDown);
            document.removeEventListener("keydown", onKeyDown);
            window.removeEventListener("scroll", onScrollOrResize, true);
            window.removeEventListener("resize", onScrollOrResize);
        };
    }, [open, close]);

    const toggle = () => {
        if (disabled) return;
        if (open) {
            close(false);
            return;
        }
        const r = btnRef.current.getBoundingClientRect();
        // Right-aligned to the button: the actions column is flush right, so
        // a left-aligned menu would hang off the edge of the window.
        setPos({ top: r.bottom + 4, right: window.innerWidth - r.right });
        setOpen(true);
    };

    // Roving focus through the menu with the arrow keys, which is what
    // role="menu" promises a screen reader user.
    const onMenuKeyDown = (e) => {
        if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
        e.preventDefault();
        const focusable = [...menuRef.current.querySelectorAll("button:not([disabled])")];
        if (focusable.length === 0) return;
        const i = focusable.indexOf(document.activeElement);
        const next = e.key === "ArrowDown" ? i + 1 : i - 1;
        focusable[(next + focusable.length) % focusable.length].focus();
    };

    return (
        <span class="menu-anchor">
            <button
                ref={btnRef}
                class={buttonClass}
                type="button"
                disabled={disabled}
                aria-haspopup="true"
                aria-expanded={open}
                aria-label={menuLabel}
                onClick={toggle}
            >
                {label} <span class="menu-caret" aria-hidden="true">▾</span>
            </button>

            {open && (
                <div
                    ref={menuRef}
                    class="menu"
                    role="menu"
                    aria-label={menuLabel}
                    style={{ top: `${pos.top}px`, right: `${pos.right}px` }}
                    onKeyDown={onMenuKeyDown}
                >
                    {items.map((item) => (
                        <button
                            key={item.key}
                            class="menu-item"
                            type="button"
                            role="menuitem"
                            disabled={item.disabled}
                            onClick={() => {
                                close(false);
                                item.onSelect();
                            }}
                        >
                            <span class="menu-item-label">{item.label}</span>
                            {item.hint && <span class="menu-item-hint">{item.hint}</span>}
                        </button>
                    ))}
                </div>
            )}
        </span>
    );
}
