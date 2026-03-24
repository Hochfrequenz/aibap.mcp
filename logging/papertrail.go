package logging

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// papertrailHandler sends JSON log records over TLS syslog (RFC 3164)
// to a Papertrail endpoint. It maintains a persistent connection and
// reconnects on failure.
type papertrailHandler struct {
	host    string
	port    string
	mu      sync.Mutex
	conn    net.Conn
	json    slog.Handler
	buf     *strings.Builder
	program string
}

func newPapertrailHandler(host, port string, level slog.Level) slog.Handler {
	buf := &strings.Builder{}
	program, _ := os.Hostname()
	if program == "" {
		program = "mcp-server-abap"
	}
	return &papertrailHandler{
		host:    host,
		port:    port,
		buf:     buf,
		program: program,
		json: slog.NewJSONHandler(buf, &slog.HandlerOptions{
			Level: level,
		}),
	}
}

func (h *papertrailHandler) connect() error {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		net.JoinHostPort(h.host, h.port),
		&tls.Config{MinVersion: tls.VersionTLS12},
	)
	if err != nil {
		return fmt.Errorf("papertrail connect: %w", err)
	}
	h.conn = conn
	return nil
}

func (h *papertrailHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.json.Enabled(ctx, level)
}

func (h *papertrailHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Render JSON into buffer.
	h.buf.Reset()
	if err := h.json.Handle(ctx, r); err != nil {
		return err
	}
	msg := strings.TrimSpace(h.buf.String())
	// Prevent newline injection in syslog.
	msg = strings.ReplaceAll(msg, "\n", " ")

	// BSD syslog format: <priority>timestamp hostname program: message
	priority := syslogPriority(r.Level)
	ts := r.Time.UTC().Format(time.RFC3339)
	line := fmt.Sprintf("<%d>%s %s %s: %s\n", priority, ts, h.program, "mcp-server-abap", msg)

	// Send, reconnecting once on failure.
	if err := h.send([]byte(line)); err != nil {
		// Reconnect and retry once.
		if h.conn != nil {
			h.conn.Close()
			h.conn = nil
		}
		if err := h.connect(); err != nil {
			return err
		}
		return h.send([]byte(line))
	}
	return nil
}

func (h *papertrailHandler) send(data []byte) error {
	if h.conn == nil {
		if err := h.connect(); err != nil {
			return err
		}
	}
	_ = h.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := h.conn.Write(data)
	return err
}

func (h *papertrailHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &papertrailHandler{
		host:    h.host,
		port:    h.port,
		buf:     &strings.Builder{},
		program: h.program,
		json:    h.json.WithAttrs(attrs),
	}
}

func (h *papertrailHandler) WithGroup(name string) slog.Handler {
	return &papertrailHandler{
		host:    h.host,
		port:    h.port,
		buf:     &strings.Builder{},
		program: h.program,
		json:    h.json.WithGroup(name),
	}
}

// syslogPriority maps slog levels to BSD syslog priority values.
// Facility 1 (user-level), severity from level.
func syslogPriority(level slog.Level) int {
	facility := 1 // user
	var severity int
	switch {
	case level >= slog.LevelError:
		severity = 3 // error
	case level >= slog.LevelWarn:
		severity = 4 // warning
	case level >= slog.LevelInfo:
		severity = 6 // informational
	default:
		severity = 7 // debug
	}
	return facility*8 + severity
}
