package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type fileConfig struct {
	HTTPAddress      optional[string] `json:"http_addr"`
	DatabaseMaxConns optional[int32]  `json:"db_max_conns"`
	LogLevel         optional[string] `json:"log_level"`
	LogFormat        optional[string] `json:"log_format"`
	JWTIssuer        optional[string] `json:"jwt_issuer"`
	JWTAudience      optional[string] `json:"jwt_audience"`
}

type optional[T any] struct {
	value T
	set   bool
}

func (value *optional[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("null is not allowed")
	}

	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	value.value = decoded
	value.set = true
	return nil
}

func loadFileConfig() (fileConfig, error) {
	path := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	if path == "" {
		return fileConfig{}, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("read configuration file %q: %w", path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()

	var values *fileConfig
	if err := decoder.Decode(&values); err != nil {
		return fileConfig{}, fmt.Errorf("decode configuration file %q: %w", path, err)
	}
	if values == nil {
		return fileConfig{}, fmt.Errorf("decode configuration file %q: top-level null is not allowed", path)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fileConfig{}, fmt.Errorf(
				"decode configuration file %q: multiple JSON values are not allowed",
				path,
			)
		}
		return fileConfig{}, fmt.Errorf("decode configuration file %q: %w", path, err)
	}

	return *values, nil
}
