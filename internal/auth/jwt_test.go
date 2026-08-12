package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	validSecret   = "0123456789abcdef0123456789abcdef"
	validIssuer   = "https://issuer.example"
	validAudience = "xm-company-service"
)

func TestValidatorAcceptsValidToken(t *testing.T) {
	validator := NewValidator(validSecret, validIssuer, validAudience)
	now := time.Now()

	tests := map[string]jwt.Claims{
		"scalar audience": jwt.MapClaims{
			"iss": validIssuer,
			"aud": validAudience,
			"exp": now.Add(time.Hour).Unix(),
		},
		"audience list containing configured audience": jwt.MapClaims{
			"iss": validIssuer,
			"aud": []string{"another-service", validAudience},
			"exp": now.Add(time.Hour).Unix(),
		},
	}

	for name, claims := range tests {
		t.Run(name, func(t *testing.T) {
			rawToken := signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))

			if err := validator.Validate(rawToken); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestValidatorRejectsInvalidTokens(t *testing.T) {
	validator := NewValidator(validSecret, validIssuer, validAudience)
	now := time.Now()
	validClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": validIssuer,
			"aud": validAudience,
			"exp": now.Add(time.Hour).Unix(),
		}
	}

	tests := map[string]func(*testing.T) string{
		"wrong secret": func(t *testing.T) string {
			return signToken(t, jwt.SigningMethodHS256, validClaims(), []byte("abcdef0123456789abcdef0123456789"))
		},
		"wrong issuer": func(t *testing.T) string {
			claims := validClaims()
			claims["iss"] = "https://different-issuer.example"
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"missing issuer": func(t *testing.T) string {
			claims := validClaims()
			delete(claims, "iss")
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"missing audience": func(t *testing.T) string {
			claims := validClaims()
			delete(claims, "aud")
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"audience list without configured audience": func(t *testing.T) string {
			claims := validClaims()
			claims["aud"] = []string{"another-service", "third-service"}
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"missing expiration": func(t *testing.T) string {
			claims := validClaims()
			delete(claims, "exp")
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"expired": func(t *testing.T) string {
			claims := validClaims()
			claims["exp"] = now.Add(-time.Minute).Unix()
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"malformed expiration": func(t *testing.T) string {
			claims := validClaims()
			claims["exp"] = "later"
			return signToken(t, jwt.SigningMethodHS256, claims, []byte(validSecret))
		},
		"HS384": func(t *testing.T) string {
			return signToken(t, jwt.SigningMethodHS384, validClaims(), []byte(validSecret))
		},
		"none": func(t *testing.T) string {
			return signToken(t, jwt.SigningMethodNone, validClaims(), jwt.UnsafeAllowNoneSignatureType)
		},
		"malformed": func(*testing.T) string {
			return "not-a-jwt"
		},
	}

	for name, makeToken := range tests {
		t.Run(name, func(t *testing.T) {
			err := validator.Validate(makeToken(t))
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestNewValidatorRequiresConfiguration(t *testing.T) {
	tests := map[string]struct {
		secret   string
		issuer   string
		audience string
	}{
		"missing secret":    {secret: "", issuer: validIssuer, audience: validAudience},
		"whitespace secret": {secret: "  ", issuer: validIssuer, audience: validAudience},
		"short secret":      {secret: "too-short", issuer: validIssuer, audience: validAudience},
		"missing issuer":    {secret: validSecret, issuer: "", audience: validAudience},
		"whitespace issuer": {secret: validSecret, issuer: "  ", audience: validAudience},
		"missing audience":  {secret: validSecret, issuer: validIssuer, audience: ""},
		"whitespace audience": {
			secret: validSecret, issuer: validIssuer, audience: "  ",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("NewValidator did not panic")
				}
			}()

			NewValidator(test.secret, test.issuer, test.audience)
		})
	}
}

func signToken(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, key any) string {
	t.Helper()

	rawToken, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return rawToken
}
