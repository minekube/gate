package gate

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

// TestEnvDisablesColor pins the accepted spellings: NO_COLOR (no-color.org) and
// GATE_NO_COLOR, with an empty value or "0" explicitly meaning "keep colour".
func TestEnvDisablesColor(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "unset", want: false},
		{name: "NO_COLOR=1", env: map[string]string{"NO_COLOR": "1"}, want: true},
		{name: "NO_COLOR=random", env: map[string]string{"NO_COLOR": "random"}, want: true},
		{name: "NO_COLOR empty keeps colour", env: map[string]string{"NO_COLOR": ""}, want: false},
		{name: "NO_COLOR=0 keeps colour", env: map[string]string{"NO_COLOR": "0"}, want: false},
		{name: "GATE_NO_COLOR=1", env: map[string]string{"GATE_NO_COLOR": "1"}, want: true},
		{name: "GATE_NO_COLOR=0 keeps colour", env: map[string]string{"GATE_NO_COLOR": "0"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv cannot unset, and an empty value is the documented
			// "no opinion" spelling for both variables, so clear them first.
			t.Setenv("NO_COLOR", "")
			t.Setenv("GATE_NO_COLOR", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := envDisablesColor(); got != tc.want {
				t.Fatalf("envDisablesColor() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLevelEncoderWritesNoEscapesWhenColourIsDisabled is the operator-facing
// contract: a redirected log file must not be full of ANSI escapes, which is
// exactly what `gate >> gate.log 2>&1` produced before.
func TestLevelEncoderWritesNoEscapesWhenColourIsDisabled(t *testing.T) {
	encode := func() string {
		t.Helper()
		enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			LevelKey:       "level",
			TimeKey:        "time",
			NameKey:        "logger",
			MessageKey:     "msg",
			EncodeLevel:    levelEncoder(),
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeName:     zapcore.FullNameEncoder,
		})
		buf, err := enc.EncodeEntry(zapcore.Entry{
			Level:      zapcore.InfoLevel,
			Time:       time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
			LoggerName: "gate",
			Message:    "logging verbosity",
		}, nil)
		if err != nil {
			t.Fatalf("EncodeEntry: %v", err)
		}
		return buf.String()
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("GATE_NO_COLOR", "")
	withColour := encode()
	if !strings.Contains(withColour, "\x1b[") {
		t.Fatalf("default encoder should keep colour, got %q", withColour)
	}

	t.Setenv("NO_COLOR", "1")
	withoutColour := encode()
	if strings.Contains(withoutColour, "\x1b[") {
		t.Fatalf("NO_COLOR output must not contain ANSI escapes, got %q", withoutColour)
	}
	if !strings.Contains(withoutColour, "INFO") {
		t.Fatalf("level must still be rendered, got %q", withoutColour)
	}
}
