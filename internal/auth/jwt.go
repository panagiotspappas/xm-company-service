// Package auth provides stateless JWT verification.
package auth

import (
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const minimumSecretBytes = 32

// ErrInvalidToken is returned for every token verification failure.
var ErrInvalidToken = errors.New("invalid token")

// Validator verifies JWT signatures and registered claims.
type Validator struct {
	secret []byte
	parser *jwt.Parser
}

// NewValidator constructs an HS256 JWT validator.
func NewValidator(secret, issuer, audience string) *Validator {
	if strings.TrimSpace(secret) == "" {
		panic("auth: JWT secret is required")
	}
	if len(secret) < minimumSecretBytes {
		panic("auth: JWT secret must be at least 32 bytes")
	}

	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		panic("auth: JWT issuer is required")
	}

	audience = strings.TrimSpace(audience)
	if audience == "" {
		panic("auth: JWT audience is required")
	}

	return &Validator{
		secret: []byte(secret),
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithExpirationRequired(),
		),
	}
}

// Validate verifies a raw JWT. All failures are deliberately normalized.
func (validator *Validator) Validate(rawToken string) error {
	claims := &jwt.RegisteredClaims{}
	token, err := validator.parser.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}

			return validator.secret, nil
		},
	)
	if err != nil || token == nil || !token.Valid {
		return ErrInvalidToken
	}

	return nil
}
