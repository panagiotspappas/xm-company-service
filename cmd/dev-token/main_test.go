package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panagiotspappas/xm-company-service/internal/auth"
	"github.com/panagiotspappas/xm-company-service/internal/config"
)

const (
	testSecret   = "0123456789abcdef0123456789abcdef"
	testIssuer   = "https://issuer.example"
	testAudience = "xm-company-service"
)

func TestGenerateTokenClaimsAndLifetime(t *testing.T) {
	issuedAt := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	lifetime := 90 * time.Minute
	jwtConfig := config.JWTConfig{
		Secret:   testSecret,
		Issuer:   testIssuer,
		Audience: testAudience,
	}

	rawToken, err := generateToken(jwtConfig, lifetime, func() time.Time { return issuedAt })
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}

	claims := &jwt.RegisteredClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(testIssuer),
		jwt.WithAudience(testAudience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return issuedAt.Add(time.Minute) }),
	)
	token, err := parser.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("parse generated token: %v", err)
	}
	if !token.Valid {
		t.Fatal("generated token is not valid")
	}
	if token.Method != jwt.SigningMethodHS256 {
		t.Fatalf("signing method = %s, want HS256", token.Method.Alg())
	}
	if claims.Issuer != testIssuer {
		t.Fatalf("issuer = %q, want %q", claims.Issuer, testIssuer)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != testAudience {
		t.Fatalf("audience = %#v, want [%s]", claims.Audience, testAudience)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("generated token is missing iat or exp")
	}
	if got := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time); got != lifetime {
		t.Fatalf("exp - iat = %s, want %s", got, lifetime)
	}
}

func TestGenerateTokenAcceptedByProductionValidator(t *testing.T) {
	jwtConfig := config.JWTConfig{
		Secret:   testSecret,
		Issuer:   testIssuer,
		Audience: testAudience,
	}
	rawToken, err := generateToken(jwtConfig, time.Hour, time.Now)
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}

	validator := auth.NewValidator(testSecret, testIssuer, testAudience)
	if err := validator.Validate(rawToken); err != nil {
		t.Fatalf("production Validate() error = %v", err)
	}
}

func TestGenerateTokenValidatesLifetime(t *testing.T) {
	jwtConfig := config.JWTConfig{
		Secret:   testSecret,
		Issuer:   testIssuer,
		Audience: testAudience,
	}
	tests := map[string]struct {
		lifetime  time.Duration
		wantError bool
	}{
		"zero":                  {lifetime: 0, wantError: true},
		"negative":              {lifetime: -time.Second, wantError: true},
		"below one second":      {lifetime: 500 * time.Millisecond, wantError: true},
		"fractional second":     {lifetime: 1500 * time.Millisecond, wantError: true},
		"one second":            {lifetime: time.Second},
		"whole-second duration": {lifetime: 90 * time.Minute},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := generateToken(jwtConfig, test.lifetime, time.Now)
			if (err != nil) != test.wantError {
				t.Fatalf("generateToken() error = %v, wantError = %t", err, test.wantError)
			}
		})
	}
}

func TestGenerateTokenRequiresClock(t *testing.T) {
	jwtConfig := config.JWTConfig{
		Secret:   testSecret,
		Issuer:   testIssuer,
		Audience: testAudience,
	}

	if _, err := generateToken(jwtConfig, time.Hour, nil); err == nil {
		t.Fatal("generateToken() error = nil, want clock error")
	}
}

func TestRunPrintsOnlyToken(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	t.Setenv("JWT_ISSUER", testIssuer)
	t.Setenv("JWT_AUDIENCE", testAudience)
	issuedAt := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		[]string{"-ttl", "90m"},
		&stdout,
		&stderr,
		func() time.Time { return issuedAt },
	)
	if err != nil {
		t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Count(stdout.String(), "\n") != 1 || !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("stdout = %q, want one token line", stdout.String())
	}
	if strings.Count(strings.TrimSpace(stdout.String()), ".") != 2 {
		t.Fatalf("stdout = %q, want compact JWT", stdout.String())
	}
}
