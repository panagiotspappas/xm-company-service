// Command dev-token generates a short-lived JWT for local development.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panagiotspappas/xm-company-service/internal/config"
)

const defaultTokenLifetime = time.Hour

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now); err != nil {
		fmt.Fprintf(os.Stderr, "generate development token: %v\n", err)
		os.Exit(1)
	}
}

func run(
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	now func() time.Time,
) error {
	flags := flag.NewFlagSet("dev-token", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lifetime := flags.Duration("ttl", defaultTokenLifetime, "token lifetime")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := validateTokenLifetime(*lifetime); err != nil {
		return err
	}

	jwtConfig, err := config.LoadJWT()
	if err != nil {
		return fmt.Errorf("load JWT configuration: %w", err)
	}

	rawToken, err := generateToken(jwtConfig, *lifetime, now)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(stdout, rawToken); err != nil {
		return fmt.Errorf("write token: %w", err)
	}

	return nil
}

func generateToken(
	jwtConfig config.JWTConfig,
	lifetime time.Duration,
	now func() time.Time,
) (string, error) {
	if err := validateTokenLifetime(lifetime); err != nil {
		return "", err
	}
	if now == nil {
		return "", errors.New("clock is required")
	}

	issuedAt := now().UTC()
	claims := jwt.RegisteredClaims{
		Issuer:    jwtConfig.Issuer,
		Audience:  jwt.ClaimStrings{jwtConfig.Audience},
		ExpiresAt: jwt.NewNumericDate(issuedAt.Add(lifetime)),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	rawToken, err := token.SignedString([]byte(jwtConfig.Secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return rawToken, nil
}

func validateTokenLifetime(lifetime time.Duration) error {
	if lifetime < time.Second || lifetime%time.Second != 0 {
		return errors.New("token lifetime must be at least one second and use whole-second precision")
	}

	return nil
}
