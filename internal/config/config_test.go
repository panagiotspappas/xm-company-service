package config

import (
	"log/slog"
	"testing"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setEnvironment(t, "", "", "", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing DATABASE_URL error")
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnvironment(t, "postgres://example", "", "", "")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != ":8080" {
		t.Fatalf("HTTPAddress = %q, want :8080", got.HTTPAddress)
	}
	if got.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelInfo)
	}
	if got.LogFormat != LogFormatText {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatText)
	}
}

func TestLoadConfiguredValues(t *testing.T) {
	setEnvironment(t, "postgres://example", "127.0.0.1:9090", "DEBUG", "json")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddress = %q, want 127.0.0.1:9090", got.HTTPAddress)
	}
	if got.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelDebug)
	}
	if got.LogFormat != LogFormatJSON {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatJSON)
	}
}

func TestLoadLogLevels(t *testing.T) {
	tests := map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			setEnvironment(t, "postgres://example", "", input, "")

			got, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.LogLevel != want {
				t.Fatalf("LogLevel = %v, want %v", got.LogLevel, want)
			}
		})
	}
}

func TestLoadRejectsInvalidLoggingConfiguration(t *testing.T) {
	t.Run("level", func(t *testing.T) {
		setEnvironment(t, "postgres://example", "", "TRACE", "")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid LOG_LEVEL error")
		}
	})

	t.Run("format", func(t *testing.T) {
		setEnvironment(t, "postgres://example", "", "", "console")

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid LOG_FORMAT error")
		}
	})
}

func setEnvironment(t *testing.T, databaseURL, httpAddress, logLevel, logFormat string) {
	t.Helper()
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("HTTP_ADDR", httpAddress)
	t.Setenv("LOG_LEVEL", logLevel)
	t.Setenv("LOG_FORMAT", logFormat)
}
