package logging

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ogen-app/ogen/src/kernel/config"
)

// New builds the application logger from config, installs it as slog's default,
// and routes the stdlib log package through it so any residual log.Print output
// (ours during the migration window, or a dependency's) is captured as a
// structured record rather than a raw stderr write. Output goes to stderr;
// format and level are configurable via LOG_FORMAT / LOG_LEVEL.
//
// Call once, as early as possible in main, so even pre-DB boot errors are
// structured.
func New(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: ParseLevel(cfg.LogLevel)}

	var base slog.Handler
	if UseJSON(cfg.LogFormat, cfg.Debug) {
		base = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		base = slog.NewTextHandler(os.Stderr, opts)
	}

	logger := slog.New(ContextHandler{Handler: base})
	slog.SetDefault(logger)

	// Bridge the stdlib logger onto slog. Drop its date/time/prefix flags —
	// slog stamps its own time — and route each line through the default
	// logger as a structured record tagged component=stdlog.
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(bridgeWriter{logger: logger})

	return logger
}

// UseJSON resolves the handler format. An explicit "json"/"text" always wins;
// empty/unknown resolves per environment — text when DEBUG=true (local
// ergonomics), JSON otherwise (production default).
func UseJSON(format string, debug bool) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return false
	case "json":
		return true
	default:
		return !debug
	}
}

// ParseLevel maps a LOG_LEVEL string to a slog.Level. Unknown/empty ⇒ info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
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

// Preview caps s to at most maxBytes bytes for safe logging of user- or
// model-supplied free text, appending an ellipsis when truncated. It trims to a
// valid UTF-8 boundary so the JSON handler never emits a split rune. Prefer this
// (or a plain length attribute) over logging unbounded content — model output
// and user instructions can contain arbitrary, sensitive data.
func Preview(s string, maxBytes int) string {
	if maxBytes < 0 {
		maxBytes = 0
	}
	if len(s) <= maxBytes {
		return s
	}
	b := s[:maxBytes]
	for len(b) > 0 && !utf8.ValidString(b) {
		b = b[:len(b)-1]
	}
	return b + "…"
}

// bridgeWriter adapts an io.Writer to slog for the stdlib log bridge. Each
// Write is one already-formatted log line; it is emitted at INFO and tagged so
// bridged output is distinguishable from native slog calls.
type bridgeWriter struct{ logger *slog.Logger }

func (w bridgeWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	w.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, slog.String(AttrComponent, "stdlog"))
	return len(p), nil
}
