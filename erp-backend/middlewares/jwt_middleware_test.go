package middlewares

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateToken_AdminTokenExpiresAfterSevenDays(t *testing.T) {
	if err := ConfigureJWTSecret("isolated-test-signing-secret-32-bytes-minimum"); err != nil {
		t.Fatal(err)
	}
	tokenString, err := GenerateToken("admin")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return JwtKey, nil
	})
	if err != nil {
		t.Fatalf("ParseWithClaims() error = %v", err)
	}
	if !token.Valid {
		t.Fatal("GenerateToken() produced an invalid token")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("GenerateToken() did not set an expiration time")
	}
	if claims.IssuedAt == nil {
		t.Fatal("GenerateToken() did not set an issued time")
	}

	if duration := claims.ExpiresAt.Sub(claims.IssuedAt.Time); duration != adminTokenDuration {
		t.Fatalf(
			"token duration = %v, want %v",
			duration,
			adminTokenDuration,
		)
	}
}

func TestConfigureJWTSecret_RejectsMissingOrShortSecret(t *testing.T) {
	for _, secret := range []string{"", "short"} {
		if err := ConfigureJWTSecret(secret); err == nil {
			t.Fatalf("ConfigureJWTSecret(%q) unexpectedly succeeded", secret)
		}
	}
}
