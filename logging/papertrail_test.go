package logging

import (
	"log/slog"
	"testing"
)

func TestSyslogPriority(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  int
	}{
		{slog.LevelDebug, 15}, // facility=1 (user), severity=7 (debug): 1*8+7=15
		{slog.LevelInfo, 14},  // 1*8+6=14
		{slog.LevelWarn, 12},  // 1*8+4=12
		{slog.LevelError, 11}, // 1*8+3=11
	}
	for _, tt := range tests {
		if got := syslogPriority(tt.level); got != tt.want {
			t.Errorf("syslogPriority(%v) = %d, want %d", tt.level, got, tt.want)
		}
	}
}
