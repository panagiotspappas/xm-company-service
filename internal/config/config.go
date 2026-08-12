package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	defaultHTTPAddress    = ":8080"
	defaultLogLevel       = "INFO"
	defaultLogFormat      = "text"
	minimumJWTSecretBytes = 32
)

// LogFormat identifies the structured log encoding.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Config contains the application's runtime configuration.
type Config struct {
	HTTPAddress string
	DatabaseURL string
	LogLevel    slog.Level
	LogFormat   LogFormat
	JWT         JWTConfig
}

// JWTConfig contains the configuration shared by JWT verification and
// development token generation.
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
}

// Load reads and validates configuration from environment variables.
func Load() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	jwtConfig, err := LoadJWT()
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(valueOrDefault("LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return Config{}, err
	}

	logFormat, err := parseLogFormat(valueOrDefault("LOG_FORMAT", defaultLogFormat))
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress: valueOrDefault("HTTP_ADDR", defaultHTTPAddress),
		DatabaseURL: databaseURL,
		LogLevel:    logLevel,
		LogFormat:   logFormat,
		JWT:         jwtConfig,
	}, nil
}

// LoadJWT reads and validates the JWT settings without loading unrelated
// application configuration.
func LoadJWT() (JWTConfig, error) {
	secret := os.Getenv("JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return JWTConfig{}, errors.New("JWT_SECRET is required")
	}
	if len(secret) < minimumJWTSecretBytes {
		return JWTConfig{}, errors.New("JWT_SECRET must be at least 32 bytes")
	}

	issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
	if issuer == "" {
		return JWTConfig{}, errors.New("JWT_ISSUER is required")
	}

	audience := strings.TrimSpace(os.Getenv("JWT_AUDIENCE"))
	if audience == "" {
		return JWTConfig{}, errors.New("JWT_AUDIENCE is required")
	}

	return JWTConfig{
		Secret:   secret,
		Issuer:   issuer,
		Audience: audience,
	}, nil
}

func valueOrDefault(name, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}

	return value
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q: expected DEBUG, INFO, WARN, or ERROR", value)
	}
}

func parseLogFormat(value string) (LogFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(LogFormatText):
		return LogFormatText, nil
	case string(LogFormatJSON):
		return LogFormatJSON, nil
	default:
		return "", fmt.Errorf("invalid LOG_FORMAT %q: expected text or json", value)
	}
}
