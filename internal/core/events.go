package core

// Event is the framework-agnostic progress/notification type emitted by
// long-running core operations (installs, launches). app.go adapts these to
// Wails runtime.EventsEmit calls; a future CLI/TUI (backlog) could instead
// print them or ignore them entirely -- core never imports Wails.
type Event struct {
	Type string         `json:"type"` // e.g. "download_progress", "install_complete"
	ID   string         `json:"id"`   // the InstalledVersion or Project ID this event is about
	Data map[string]any `json:"data"` // event-specific payload
}

// EventSink receives Events emitted by core operations. Implemented by
// app.go's Wails adapter in v1.
type EventSink interface {
	Emit(Event)
}
