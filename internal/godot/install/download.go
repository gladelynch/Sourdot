package install

// Progress-tracked download: an io.Reader wrapper that counts bytes read
// and emits core.Event{Type: "download_progress"} at a throttled interval
// (~every 1%/100ms, to avoid flooding the Wails event bridge). Implemented
// in M1.
