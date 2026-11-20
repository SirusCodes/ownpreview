package utils_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"ownpreview/internal/utils"
)

func TestNewLoggerDoesNotModifyDefaultLogger(t *testing.T) {
	initialDefault := slog.Default()

	utils.ConfigureLogger("debug", "json")
	logger := utils.NewLogger("test-component")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	if slog.Default() != initialDefault {
		t.Errorf("expected slog.Default() to remain untouched")
	}

	// Reset configuration
	utils.ConfigureLogger("info", "text")
}

func TestNewLoggerComponentAndFormat(t *testing.T) {
	buf := new(bytes.Buffer)
	utils.SetLogOutput(buf)
	defer utils.SetLogOutput(os.Stderr)

	utils.ConfigureLogger("debug", "json")
	defer utils.ConfigureLogger("info", "text")

	logger := utils.NewLogger("my-component")
	logger.DebugContext(context.Background(), "test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, `"component":"my-component"`) {
		t.Errorf("expected component in json log output, got: %s", output)
	}
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("expected msg in json log output, got: %s", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("expected key in json log output, got: %s", output)
	}
}

func TestConsoleFormatIsCleanAndLowNoise(t *testing.T) {
	buf := new(bytes.Buffer)
	utils.SetLogOutput(buf)
	defer utils.SetLogOutput(os.Stderr)

	utils.ConfigureLogger("info", "text")
	logger := utils.NewLogger("builder")

	logger.InfoContext(context.Background(), "starting build", "image", "my-app:v1")

	output := buf.String()
	expected := "INFO [builder] starting build image=my-app:v1\n"
	if output != expected {
		t.Errorf("expected clean console output %q, got %q", expected, output)
	}

	// Ensure no noisy timestamps or msg= wrappers
	if strings.Contains(output, "time=") || strings.Contains(output, "msg=") {
		t.Errorf("console output should not contain timestamp or raw msg= keys: %s", output)
	}
}

func TestLogLevelFiltering(t *testing.T) {
	buf := new(bytes.Buffer)
	utils.SetLogOutput(buf)
	defer utils.SetLogOutput(os.Stderr)

	utils.ConfigureLogger("info", "text")
	logger := utils.NewLogger("filter-component")

	logger.DebugContext(context.Background(), "debug should be ignored")
	if buf.Len() > 0 {
		t.Errorf("expected debug log to be filtered out at info level, got: %s", buf.String())
	}

	logger.InfoContext(context.Background(), "info should be logged")
	if !strings.Contains(buf.String(), "info should be logged") {
		t.Errorf("expected info message to appear, got: %s", buf.String())
	}
}
