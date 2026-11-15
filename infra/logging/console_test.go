package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func restoreConsole(t *testing.T) {
	t.Helper()
	oldOut, oldLevel := consoleOut, consoleLevel
	t.Cleanup(func() {
		consoleOut, consoleLevel = oldOut, oldLevel
		ConsoleLog = consoleLogger(consoleLevel)
	})
}

func TestSetConsoleOutputRedirects(t *testing.T) {
	restoreConsole(t)

	var buf bytes.Buffer
	SetConsoleOutput(&buf)
	ConsoleLog.Info().Msg("to-buffer")

	if !strings.Contains(buf.String(), "to-buffer") {
		t.Fatalf("redirected output = %q", buf.String())
	}
}

func TestSetConsoleOutputKeepsLevel(t *testing.T) {
	restoreConsole(t)

	SetLevel(zerolog.WarnLevel)
	var buf bytes.Buffer
	SetConsoleOutput(&buf)
	ConsoleLog.Info().Msg("dropped")
	ConsoleLog.Warn().Msg("kept")

	got := buf.String()
	if strings.Contains(got, "dropped") || !strings.Contains(got, "kept") {
		t.Fatalf("level was reset by SetConsoleOutput: %q", got)
	}
}

func TestSetLevelKeepsOutput(t *testing.T) {
	restoreConsole(t)

	var buf bytes.Buffer
	SetConsoleOutput(&buf)
	SetLevel(zerolog.DebugLevel)
	ConsoleLog.Debug().Msg("debug-line")

	if !strings.Contains(buf.String(), "debug-line") {
		t.Fatalf("output was reset by SetLevel: %q", buf.String())
	}
}

func TestSetConsoleOutputNilFallsBackToStdout(t *testing.T) {
	restoreConsole(t)

	SetConsoleOutput(nil)
	if consoleOut != os.Stdout {
		t.Fatal("nil writer must fall back to os.Stdout")
	}
}
