package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testJWTSecret   = "0123456789abcdef0123456789abcdef"
	testJWTIssuer   = "https://issuer.example"
	testJWTAudience = "xm-company-service"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setEnvironment(t, "", "", "", "", testJWTSecret, testJWTIssuer, testJWTAudience)

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing DATABASE_URL error")
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnvironment(
		t,
		"postgres://example",
		"",
		"",
		"",
		testJWTSecret,
		testJWTIssuer,
		testJWTAudience,
	)
	t.Setenv("CONFIG_FILE", "   ")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != ":8080" {
		t.Fatalf("HTTPAddress = %q, want :8080", got.HTTPAddress)
	}
	if got.DatabaseMaxConns != 10 {
		t.Fatalf("DatabaseMaxConns = %d, want 10", got.DatabaseMaxConns)
	}
	if got.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelInfo)
	}
	if got.LogFormat != LogFormatText {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatText)
	}
	if got.JWT.Secret != testJWTSecret {
		t.Fatal("JWT.Secret was not loaded")
	}
	if got.JWT.Issuer != testJWTIssuer {
		t.Fatalf("JWT.Issuer = %q, want %q", got.JWT.Issuer, testJWTIssuer)
	}
	if got.JWT.Audience != testJWTAudience {
		t.Fatalf("JWT.Audience = %q, want %q", got.JWT.Audience, testJWTAudience)
	}
}

func TestLoadConfiguredValues(t *testing.T) {
	setEnvironment(
		t,
		"postgres://example",
		"127.0.0.1:9090",
		"DEBUG",
		"json",
		testJWTSecret,
		"  "+testJWTIssuer+"  ",
		"  "+testJWTAudience+"  ",
	)
	t.Setenv("DB_MAX_CONNS", "25")

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
	if got.DatabaseMaxConns != 25 {
		t.Fatalf("DatabaseMaxConns = %d, want 25", got.DatabaseMaxConns)
	}
	if got.JWT.Issuer != testJWTIssuer {
		t.Fatalf("JWT.Issuer = %q, want trimmed value %q", got.JWT.Issuer, testJWTIssuer)
	}
	if got.JWT.Audience != testJWTAudience {
		t.Fatalf("JWT.Audience = %q, want trimmed value %q", got.JWT.Audience, testJWTAudience)
	}
}

func TestLoadConfigurationFile(t *testing.T) {
	setEnvironment(t, "postgres://example", "", "", "", testJWTSecret, "", "")
	path := writeConfigurationFile(t, `{
		"http_addr": " 127.0.0.1:9090 ",
		"db_max_conns": 25,
		"log_level": "debug",
		"log_format": "JSON",
		"jwt_issuer": " file-issuer ",
		"jwt_audience": " file-audience "
	}`)
	t.Setenv("CONFIG_FILE", "  "+path+"  ")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddress = %q, want 127.0.0.1:9090", got.HTTPAddress)
	}
	if got.DatabaseMaxConns != 25 {
		t.Fatalf("DatabaseMaxConns = %d, want 25", got.DatabaseMaxConns)
	}
	if got.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelDebug)
	}
	if got.LogFormat != LogFormatJSON {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatJSON)
	}
	if got.JWT.Secret != testJWTSecret {
		t.Fatal("JWT.Secret was not loaded from the environment")
	}
	if got.JWT.Issuer != "file-issuer" {
		t.Fatalf("JWT.Issuer = %q, want file-issuer", got.JWT.Issuer)
	}
	if got.JWT.Audience != "file-audience" {
		t.Fatalf("JWT.Audience = %q, want file-audience", got.JWT.Audience)
	}
}

func TestLoadConfigurationFileEnvironmentOverrides(t *testing.T) {
	setEnvironment(
		t,
		"postgres://example",
		"127.0.0.1:9191",
		"WARN",
		"text",
		testJWTSecret,
		"environment-issuer",
		"environment-audience",
	)
	t.Setenv("DB_MAX_CONNS", "30")
	t.Setenv("CONFIG_FILE", writeConfigurationFile(t, `{
		"http_addr": "127.0.0.1:9090",
		"db_max_conns": 20,
		"log_level": "DEBUG",
		"log_format": "json",
		"jwt_issuer": "file-issuer",
		"jwt_audience": "file-audience"
	}`))

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != "127.0.0.1:9191" {
		t.Fatalf("HTTPAddress = %q, want environment override", got.HTTPAddress)
	}
	if got.DatabaseMaxConns != 30 {
		t.Fatalf("DatabaseMaxConns = %d, want 30", got.DatabaseMaxConns)
	}
	if got.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelWarn)
	}
	if got.LogFormat != LogFormatText {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatText)
	}
	if got.JWT.Issuer != "environment-issuer" {
		t.Fatalf("JWT.Issuer = %q, want environment-issuer", got.JWT.Issuer)
	}
	if got.JWT.Audience != "environment-audience" {
		t.Fatalf("JWT.Audience = %q, want environment-audience", got.JWT.Audience)
	}
}

func TestLoadPartialConfigurationFileKeepsDefaults(t *testing.T) {
	setEnvironment(t, "postgres://example", "", "", "", testJWTSecret, "", "")
	t.Setenv("CONFIG_FILE", writeConfigurationFile(t, `{
		"jwt_issuer": "file-issuer",
		"jwt_audience": "file-audience"
	}`))

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddress != defaultHTTPAddress {
		t.Fatalf("HTTPAddress = %q, want %q", got.HTTPAddress, defaultHTTPAddress)
	}
	if got.DatabaseMaxConns != defaultDatabaseMaxConns {
		t.Fatalf("DatabaseMaxConns = %d, want %d", got.DatabaseMaxConns, defaultDatabaseMaxConns)
	}
	if got.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelInfo)
	}
	if got.LogFormat != LogFormatText {
		t.Fatalf("LogFormat = %q, want %q", got.LogFormat, LogFormatText)
	}
}

func TestLoadRejectsInvalidConfigurationFile(t *testing.T) {
	tests := map[string]string{
		"empty":                  "",
		"malformed":              `{`,
		"top-level array":        `[]`,
		"top-level null":         `null`,
		"unknown field":          `{"unexpected":true}`,
		"database URL":           `{"database_url":"postgres://file"}`,
		"JWT secret":             `{"jwt_secret":"file-secret"}`,
		"invalid type":           `{"db_max_conns":"10"}`,
		"explicit field null":    `{"log_level":null}`,
		"multiple JSON values":   `{} {}`,
		"trailing invalid value": `{} trailing`,
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			setEnvironment(
				t,
				"postgres://example",
				"",
				"",
				"",
				testJWTSecret,
				testJWTIssuer,
				testJWTAudience,
			)
			t.Setenv("CONFIG_FILE", writeConfigurationFile(t, contents))

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want configuration-file error")
			}
			if !strings.Contains(err.Error(), "configuration file") {
				t.Fatalf("Load() error = %q, want configuration-file context", err)
			}
		})
	}
}

func TestLoadRejectsMissingConfigurationFile(t *testing.T) {
	setEnvironment(
		t,
		"postgres://example",
		"",
		"",
		"",
		testJWTSecret,
		testJWTIssuer,
		testJWTAudience,
	)
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "missing.json"))

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unreadable configuration-file error")
	}
	if !strings.Contains(err.Error(), "read configuration file") {
		t.Fatalf("Load() error = %q, want read context", err)
	}
}

func TestLoadRejectsInvalidConfigurationFileValues(t *testing.T) {
	tests := map[string]string{
		"zero database connections": `{"db_max_conns":0}`,
		"negative connections":      `{"db_max_conns":-1}`,
		"invalid log level":         `{"log_level":"TRACE"}`,
		"invalid log format":        `{"log_format":"console"}`,
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			setEnvironment(
				t,
				"postgres://example",
				"",
				"",
				"",
				testJWTSecret,
				testJWTIssuer,
				testJWTAudience,
			)
			t.Setenv("CONFIG_FILE", writeConfigurationFile(t, contents))

			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want invalid merged configuration error")
			}
		})
	}
}

func TestLoadRejectsInvalidEnvironmentOverride(t *testing.T) {
	setEnvironment(
		t,
		"postgres://example",
		"",
		"",
		"",
		testJWTSecret,
		testJWTIssuer,
		testJWTAudience,
	)
	t.Setenv("CONFIG_FILE", writeConfigurationFile(t, `{"db_max_conns":20}`))
	t.Setenv("DB_MAX_CONNS", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid environment override error")
	}
}

func TestLoadDatabaseMaxConns(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int32
	}{
		{name: "empty", value: "", want: 10},
		{name: "whitespace only", value: "   ", want: 10},
		{name: "default value", value: "10", want: 10},
		{name: "surrounding whitespace", value: " 10 ", want: 10},
		{name: "lower boundary", value: "1", want: 1},
		{name: "configured value", value: "25", want: 25},
		{name: "upper boundary", value: "2147483647", want: 2147483647},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setEnvironment(
				t,
				"postgres://example",
				"",
				"",
				"",
				testJWTSecret,
				testJWTIssuer,
				testJWTAudience,
			)
			t.Setenv("DB_MAX_CONNS", test.value)

			got, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.DatabaseMaxConns != test.want {
				t.Fatalf("DatabaseMaxConns = %d, want %d", got.DatabaseMaxConns, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidDatabaseMaxConns(t *testing.T) {
	tests := map[string]string{
		"zero":       "0",
		"negative":   "-1",
		"fractional": "1.5",
		"malformed":  "abc",
		"overflow":   "2147483648",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			setEnvironment(
				t,
				"postgres://example",
				"",
				"",
				"",
				testJWTSecret,
				testJWTIssuer,
				testJWTAudience,
			)
			t.Setenv("DB_MAX_CONNS", value)

			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want invalid DB_MAX_CONNS error")
			}
		})
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
			setEnvironment(
				t,
				"postgres://example",
				"",
				input,
				"",
				testJWTSecret,
				testJWTIssuer,
				testJWTAudience,
			)

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
		setEnvironment(
			t,
			"postgres://example",
			"",
			"TRACE",
			"",
			testJWTSecret,
			testJWTIssuer,
			testJWTAudience,
		)

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid LOG_LEVEL error")
		}
	})

	t.Run("format", func(t *testing.T) {
		setEnvironment(
			t,
			"postgres://example",
			"",
			"",
			"console",
			testJWTSecret,
			testJWTIssuer,
			testJWTAudience,
		)

		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid LOG_FORMAT error")
		}
	})
}

func TestLoadJWTRejectsInvalidConfiguration(t *testing.T) {
	tests := map[string]struct {
		secret   string
		issuer   string
		audience string
	}{
		"missing secret":      {secret: "", issuer: testJWTIssuer, audience: testJWTAudience},
		"whitespace secret":   {secret: "   ", issuer: testJWTIssuer, audience: testJWTAudience},
		"short secret":        {secret: "too-short", issuer: testJWTIssuer, audience: testJWTAudience},
		"missing issuer":      {secret: testJWTSecret, issuer: "", audience: testJWTAudience},
		"whitespace issuer":   {secret: testJWTSecret, issuer: "  ", audience: testJWTAudience},
		"missing audience":    {secret: testJWTSecret, issuer: testJWTIssuer, audience: ""},
		"whitespace audience": {secret: testJWTSecret, issuer: testJWTIssuer, audience: "  "},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			setJWTEnvironment(t, test.secret, test.issuer, test.audience)

			if _, err := LoadJWT(); err == nil {
				t.Fatal("LoadJWT() error = nil, want configuration error")
			}
		})
	}
}

func TestLoadJWTPreservesSecretAndTrimsClaimsConfiguration(t *testing.T) {
	secret := "  0123456789abcdef0123456789abcdef  "
	setJWTEnvironment(t, secret, "  "+testJWTIssuer+"  ", "  "+testJWTAudience+"  ")

	got, err := LoadJWT()
	if err != nil {
		t.Fatalf("LoadJWT() error = %v", err)
	}
	if got.Secret != secret {
		t.Fatalf("Secret = %q, want raw value %q", got.Secret, secret)
	}
	if got.Issuer != testJWTIssuer {
		t.Fatalf("Issuer = %q, want %q", got.Issuer, testJWTIssuer)
	}
	if got.Audience != testJWTAudience {
		t.Fatalf("Audience = %q, want %q", got.Audience, testJWTAudience)
	}
}

func TestLoadJWTUsesConfigurationFileWithoutDatabaseConfiguration(t *testing.T) {
	setJWTEnvironment(t, testJWTSecret, "", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("CONFIG_FILE", writeConfigurationFile(t, `{
		"db_max_conns": 0,
		"jwt_issuer": " file-issuer ",
		"jwt_audience": " file-audience "
	}`))

	got, err := LoadJWT()
	if err != nil {
		t.Fatalf("LoadJWT() error = %v", err)
	}
	if got.Secret != testJWTSecret {
		t.Fatal("JWT.Secret was not loaded from the environment")
	}
	if got.Issuer != "file-issuer" {
		t.Fatalf("Issuer = %q, want file-issuer", got.Issuer)
	}
	if got.Audience != "file-audience" {
		t.Fatalf("Audience = %q, want file-audience", got.Audience)
	}
}

func setEnvironment(
	t *testing.T,
	databaseURL,
	httpAddress,
	logLevel,
	logFormat,
	jwtSecret,
	jwtIssuer,
	jwtAudience string,
) {
	t.Helper()
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("HTTP_ADDR", httpAddress)
	t.Setenv("LOG_LEVEL", logLevel)
	t.Setenv("LOG_FORMAT", logFormat)
	t.Setenv("DB_MAX_CONNS", "")
	setJWTEnvironment(t, jwtSecret, jwtIssuer, jwtAudience)
}

func setJWTEnvironment(t *testing.T, secret, issuer, audience string) {
	t.Helper()
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("JWT_SECRET", secret)
	t.Setenv("JWT_ISSUER", issuer)
	t.Setenv("JWT_AUDIENCE", audience)
}

func writeConfigurationFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write configuration file: %v", err)
	}
	return path
}
