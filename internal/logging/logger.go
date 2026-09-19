// Package logging provides the process logger and secret redaction registry.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Record is a single persisted log line (used by the in-DB log viewer).
type Record struct {
	Time    time.Time
	Level   string
	RID     string
	API     string
	Message string
}

// Sink receives every log record for optional persistence.
type Sink interface {
	Write(Record)
}

// Logger writes structured logs in the serpent-shim line format:
//
//	15:04:05.000 INFO  [0001-9f3a|apiserpent] message key=value
//
// Secrets registered in the Redactor are masked before output.
type Logger struct {
	h *handler
}

// Options configures a Logger.
type Options struct {
	Writer io.Writer
	Level  string // debug|info|warn|error
	JSON   bool
	Redact *Redactor
	Sinks  []Sink
}

// New builds a Logger.
func New(opts Options) *Logger {
	if opts.Writer == nil {
		opts.Writer = io.Discard
	}
	if opts.Redact == nil {
		opts.Redact = NewRedactor()
	}
	h := &handler{
		mu:     &sync.Mutex{},
		w:      opts.Writer,
		level:  parseLevel(opts.Level),
		redact: opts.Redact,
		json:   opts.JSON,
		sinks:  opts.Sinks,
	}
	return &Logger{h: h}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Redactor exposes the secret registry so callers can register credentials.
func (l *Logger) Redactor() *Redactor { return l.h.redact }

// AddSink registers an additional log sink after construction.
func (l *Logger) AddSink(s Sink) {
	l.h.mu.Lock()
	l.h.sinks = append(l.h.sinks, s)
	l.h.mu.Unlock()
}

// Redact masks registered secrets in an arbitrary string.
func (l *Logger) Redact(s string) string { return l.h.redact.String(s) }

// AddSecret registers a secret value for masking.
func (l *Logger) AddSecret(values ...string) { l.h.redact.Add(values...) }

// Debug logs at debug level.
func (l *Logger) Debug(rid, api, msg string, args ...any) {
	l.emit(slog.LevelDebug, rid, api, msg, args...)
}

// Info logs at info level.
func (l *Logger) Info(rid, api, msg string, args ...any) {
	l.emit(slog.LevelInfo, rid, api, msg, args...)
}

// Warn logs at warn level.
func (l *Logger) Warn(rid, api, msg string, args ...any) {
	l.emit(slog.LevelWarn, rid, api, msg, args...)
}

// Error logs at error level.
func (l *Logger) Error(rid, api, msg string, args ...any) {
	l.emit(slog.LevelError, rid, api, msg, args...)
}

func (l *Logger) emit(level slog.Level, rid, api, msg string, args ...any) {
	if !l.h.Enabled(context.Background(), level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("rid", rid),
		slog.String("api", api),
	}
	// Pair up key/value args defensively; a dangling key becomes an empty value.
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprint(args[i])
		var value any = ""
		if i+1 < len(args) {
			value = args[i+1]
		}
		attrs = append(attrs, slog.Any(key, value))
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(attrs...)
	_ = l.h.Handle(context.Background(), r)
}

// handler is a minimal slog.Handler producing the required line layout.
type handler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Level
	redact *Redactor
	json   bool
	sinks  []Sink
	groups []string
	attrs  []slog.Attr
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	rid, api := "", ""
	var kv []string
	collect := func(a slog.Attr) bool {
		key := a.Key
		if len(h.groups) > 0 {
			key = strings.Join(h.groups, ".") + "." + key
		}
		switch key {
		case "rid":
			rid = a.Value.String()
			return true
		case "api":
			api = a.Value.String()
			return true
		}
		kv = append(kv, key+"="+formatValue(a.Value))
		return true
	}
	for _, a := range h.attrs {
		collect(a)
	}
	r.Attrs(collect)

	message := r.Message
	message = h.redact.String(message)
	tag := rid
	if api != "" {
		tag += "|" + api
	}

	var line string
	if h.json {
		// Emit a compact JSON line; keys/message are redacted before writing.
		parts := []string{
			fmt.Sprintf("%q:%q", "time", r.Time.Format(time.RFC3339Nano)),
			fmt.Sprintf("%q:%q", "level", strings.ToLower(r.Level.String())),
			fmt.Sprintf("%q:%q", "rid", rid),
			fmt.Sprintf("%q:%q", "api", api),
			fmt.Sprintf("%q:%q", "msg", message),
		}
		for _, item := range kv {
			parts = append(parts, fmt.Sprintf("%q:%q", "kv", h.redact.String(item)))
		}
		line = "{" + strings.Join(parts, ",") + "}\n"
	} else {
		var sb strings.Builder
		sb.WriteString(r.Time.Format("15:04:05.000"))
		sb.WriteByte(' ')
		sb.WriteString(strings.ToUpper(r.Level.String()))
		sb.WriteString(" [")
		sb.WriteString(tag)
		sb.WriteString("] ")
		sb.WriteString(message)
		for _, item := range kv {
			sb.WriteByte(' ')
			sb.WriteString(h.redact.String(item))
		}
		line = sb.String() + "\n"
	}

	h.mu.Lock()
	_, _ = io.WriteString(h.w, line)
	h.mu.Unlock()

	for _, s := range h.sinks {
		s.Write(Record{
			Time:    r.Time,
			Level:   strings.ToLower(r.Level.String()),
			RID:     rid,
			API:     api,
			Message: h.redact.String(strings.TrimSpace(message + " " + strings.Join(kv, " "))),
		})
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}

func (h *handler) WithGroup(name string) slog.Handler {
	nh := *h
	nh.groups = append(append([]string{}, h.groups...), name)
	return &nh
}

func formatValue(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n") {
			return fmt.Sprintf("%q", s)
		}
		return s
	default:
		return fmt.Sprint(v.Any())
	}
}
