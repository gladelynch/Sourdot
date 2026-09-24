// Package applog gives Sourdot one durable log file.
//
// Everything the app has to say -- stdlib log, Wails' own runtime logging,
// Go-side binding failures and errors caught in the webview -- lands in
// sourdot.log next to the database, and is mirrored to stderr for anyone
// running from a terminal. Before this existed all of it went to stderr
// only, which meant it vanished with the throwaway terminal window the
// desktop launcher opens: a user hitting a bug had nothing to attach to a
// report, and neither did anyone helping them debug it.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogFileName is the log's basename inside the config directory.
const LogFileName = "sourdot.log"

// maxSize is the size at which the log is rolled aside to sourdot.log.1 on
// the next start. Rotation happens at startup rather than mid-write because
// a single run is the unit anyone actually reads back, and one previous run
// is enough history for a bug report -- Sourdot logs at DEBUG, so an
// unbounded file in the user's config directory would not stay small.
const maxSize = 5 << 20 // 5 MiB

// Logger writes timestamped, levelled lines to the log file and stderr. It
// satisfies Wails' logger.Logger interface, so passing it in options.App
// captures the framework's own output in the same file, in the same format.
type Logger struct {
	mu   sync.Mutex
	f    *os.File // nil when only stderr is available
	path string
}

// Init opens (creating or appending to) the log file in dir, rotating it
// first if it has grown past maxSize, and redirects the stdlib log package
// at it so the existing log.Printf calls are captured too.
//
// A nil error is not required for the app to run: on failure the caller
// should still use Stderr() so logging degrades rather than disappears.
func Init(dir string) (*Logger, error) {
	path := filepath.Join(dir, LogFileName)

	if fi, err := os.Stat(path); err == nil && fi.Size() > maxSize {
		// A failed rename is not worth refusing to start over; the worst
		// case is one oversized file that gets rotated next launch.
		_ = os.Rename(path, path+".1")
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	l := &Logger{f: f, path: path}
	l.adoptStdlib()
	return l, nil
}

// Stderr returns a Logger that writes nowhere but stderr, for when the log
// file can't be opened.
func Stderr() *Logger {
	l := &Logger{}
	l.adoptStdlib()
	return l
}

// adoptStdlib routes the log package through this Logger. Flags are cleared
// because write() supplies its own timestamp; leaving LstdFlags on would
// stamp every line twice.
func (l *Logger) adoptStdlib() {
	log.SetFlags(0)
	log.SetOutput(l.Writer("INFO"))
}

// Path is the absolute path of the log file, or "" if only stderr is in use.
func (l *Logger) Path() string { return l.path }

// Close flushes and releases the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// write emits one record. Multi-line messages (Go error chains, JS stack
// traces) get every line prefixed, so a later line can never be mistaken
// for the start of a new record.
func (l *Logger) write(level, message string) {
	stamp := time.Now().Format("2006-01-02 15:04:05.000")

	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(message, "\n"), "\n") {
		fmt.Fprintf(&b, "%s %-5s | %s\n", stamp, level, line)
	}
	out := b.String()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_, _ = l.f.WriteString(out)
	}
	_, _ = io.WriteString(os.Stderr, out)
}

// Writer adapts the Logger to io.Writer at a fixed level, for the stdlib
// log package and anything else that wants a sink.
func (l *Logger) Writer(level string) io.Writer { return levelWriter{l: l, level: level} }

type levelWriter struct {
	l     *Logger
	level string
}

func (w levelWriter) Write(p []byte) (int, error) {
	w.l.write(w.level, string(p))
	return len(p), nil
}

// Printf is the convenience form used across the app.
func (l *Logger) Printf(format string, args ...any) { l.write("INFO", fmt.Sprintf(format, args...)) }

// Errorf logs a failure.
func (l *Logger) Errorf(format string, args ...any) { l.write("ERROR", fmt.Sprintf(format, args...)) }

// The remaining methods satisfy Wails' logger.Logger.

func (l *Logger) Print(message string)   { l.write("INFO", message) }
func (l *Logger) Trace(message string)   { l.write("TRACE", message) }
func (l *Logger) Debug(message string)   { l.write("DEBUG", message) }
func (l *Logger) Info(message string)    { l.write("INFO", message) }
func (l *Logger) Warning(message string) { l.write("WARN", message) }
func (l *Logger) Error(message string)   { l.write("ERROR", message) }

// Fatal matches Wails' contract for its own logger: the framework calls it
// only for conditions it can't continue past, and expects the process to
// end.
func (l *Logger) Fatal(message string) {
	l.write("FATAL", message)
	_ = l.Close()
	os.Exit(1)
}
