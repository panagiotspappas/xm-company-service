package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHTTPAddress      = ":8080"
	defaultDatabaseMaxConns = int32(10)
	defaultLogLevel         = "INFO"
	defaultLogFormat        = "text"
	minimumJWTSecretBytes   = 32
	maximumDatabaseMaxConns = int64(1<<31 - 1)
)

// LogFormat identifies the structured log encoding.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Config contains the application's runtime configuration.
type Config struct {
	HTTPAddress      string
	DatabaseURL      string
	DatabaseMaxConns int32
	LogLevel         slog.Level
	LogFormat        LogFormat
	JWT              JWTConfig
}

// JWTConfig contains the configuration shared by JWT verification and
// development token generation.
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
}

// Load reads and validates configuration from the optional file and environment.
func Load() (Config, error) {
	fileValues, err := loadFileConfig()
	if err != nil {
		return Config{}, err
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	databaseMaxConnsValue := strings.TrimSpace(os.Getenv("DB_MAX_CONNS"))
	if fileValues.DatabaseMaxConns.set {
		databaseMaxConnsValue = strconv.FormatInt(int64(fileValues.DatabaseMaxConns.value), 10)
	}
	databaseMaxConns, err := parseDatabaseMaxConns(databaseMaxConnsValue)
	if err != nil {
		return Config{}, err
	}

	jwtConfig, err := loadJWT(fileValues)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(mergedString(
		defaultLogLevel,
		fileValues.LogLevel,
		"LOG_LEVEL",
	))
	if err != nil {
		return Config{}, err
	}

	logFormat, err := parseLogFormat(mergedString(
		defaultLogFormat,
		fileValues.LogFormat,
		"LOG_FORMAT",
	))
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress: mergedString(
			defaultHTTPAddress,
			fileValues.HTTPAddress,
			"HTTP_ADDR",
		),
		DatabaseURL:      databaseURL,
		DatabaseMaxConns: databaseMaxConns,
		LogLevel:         logLevel,
		LogFormat:        logFormat,
		JWT:              jwtConfig,
	}, nil
}

// LoadJWT reads and validates the JWT settings without loading unrelated
// application configuration.
func LoadJWT() (JWTConfig, error) {
	fileValues, err := loadFileConfig()
	if err != nil {
		return JWTConfig{}, err
	}

	return loadJWT(fileValues)
}

func loadJWT(fileValues fileConfig) (JWTConfig, error) {
	secret := os.Getenv("JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return JWTConfig{}, errors.New("JWT_SECRET is required")
	}
	if len(secret) < minimumJWTSecretBytes {
		return JWTConfig{}, errors.New("JWT_SECRET must be at least 32 bytes")
	}

	issuer := mergedString("", fileValues.JWTIssuer, "JWT_ISSUER")
	if issuer == "" {
		return JWTConfig{}, errors.New("JWT_ISSUER is required")
	}

	audience := mergedString("", fileValues.JWTAudience, "JWT_AUDIENCE")
	if audience == "" {
		return JWTConfig{}, errors.New("JWT_AUDIENCE is required")
	}

	return JWTConfig{
		Secret:   secret,
		Issuer:   issuer,
		Audience: audience,
	}, nil
}

func mergedString(defaultValue string, fileValue optional[string], environmentName string) string {
	value := defaultValue
	if environmentValue := strings.TrimSpace(os.Getenv(environmentName)); environmentValue != "" {
		value = environmentValue
	}
	if fileValue.set {
		if configuredValue := strings.TrimSpace(fileValue.value); configuredValue != "" {
			value = configuredValue
		}
	}

	return value
}

func parseDatabaseMaxConns(value string) (int32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultDatabaseMaxConns, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf(
			"DB_MAX_CONNS must be an integer between 1 and %d: %w",
			maximumDatabaseMaxConns,
			err,
		)
	}
	if parsed < 1 {
		return 0, fmt.Errorf(
			"DB_MAX_CONNS must be an integer between 1 and %d",
			maximumDatabaseMaxConns,
		)
	}

	return int32(parsed), nil
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
