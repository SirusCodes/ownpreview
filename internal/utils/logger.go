package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	logMu       sync.RWMutex
	logLevelVar = new(slog.LevelVar) // defaults to slog.LevelInfo (0)
	logFormat   = "text"
	logOutput   io.Writer = os.Stderr
)

// ConfigureLogger configures the log level and format for loggers created by NewLogger.
// It does NOT update the default logger (slog.Default()).
func ConfigureLogger(levelStr, formatStr string) {
	logMu.Lock()
	defer logMu.Unlock()

	logLevelVar.Set(ParseLogLevel(levelStr))

	format := strings.ToLower(strings.TrimSpace(formatStr))
	if format == "json" {
		logFormat = "json"
	} else {
		logFormat = "text"
	}
}

// SetLogOutput sets the destination writer for log output (primarily for testing).
func SetLogOutput(w io.Writer) {
	logMu.Lock()
	defer logMu.Unlock()
	logOutput = w
}

// ParseLogLevel parses a string representation of a log level into slog.Level.
func ParseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger creates an independent *slog.Logger tagged with the given component name.
// When using text format, it uses a clean, low-noise console handler that omits verbose
// timestamp prefixes and key boilerplate.
func NewLogger(component string) *slog.Logger {
	logMu.RLock()
	format := logFormat
	out := logOutput
	logMu.RUnlock()

	opts := &slog.HandlerOptions{
		Level: logLevelVar,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = newConsoleHandler(out, opts)
	}

	logger := slog.New(handler)
	if component != "" {
		logger = logger.With("component", component)
	}
	return logger
}

// consoleHandler is a lightweight, human-friendly slog.Handler designed for terminal output.
// It eliminates noisy ISO timestamps and raw key prefixes (like time=, msg=), formatting logs cleanly:
//
//	INFO [component] message key=val
type consoleHandler struct {
	opts   slog.HandlerOptions
	out    io.Writer
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func newConsoleHandler(out io.Writer, opts *slog.HandlerOptions) *consoleHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &consoleHandler{
		opts: *opts,
		out:  out,
		mu:   &sync.Mutex{},
	}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &consoleHandler{
		opts:   h.opts,
		out:    h.out,
		mu:     h.mu,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &consoleHandler{
		opts:   h.opts,
		out:    h.out,
		mu:     h.mu,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	var buf strings.Builder

	buf.WriteString(r.Level.String())
	buf.WriteByte(' ')

	var component string
	attrsToPrint := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())

	extractAttr := func(a slog.Attr) {
		if a.Key == "component" && component == "" {
			component = a.Value.String()
		} else if a.Key != "" {
			attrsToPrint = append(attrsToPrint, a)
		}
	}

	for _, a := range h.attrs {
		extractAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		extractAttr(a)
		return true
	})

	if component != "" {
		buf.WriteByte('[')
		buf.WriteString(component)
		buf.WriteString("] ")
	}

	buf.WriteString(r.Message)

	for _, a := range attrsToPrint {
		buf.WriteByte(' ')
		buf.WriteString(a.Key)
		buf.WriteByte('=')
		valStr := a.Value.String()
		if strings.ContainsAny(valStr, " \t\n\"") {
			buf.WriteString(fmt.Sprintf("%q", valStr))
		} else {
			buf.WriteString(valStr)
		}
	}
	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write([]byte(buf.String()))
	return err
}
