package auth

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	testSecret   = "test-secret-key"
	wrongSecret  = "wrong-secret-key"
	testLifetime = time.Hour
)

func TestMakeJWT(t *testing.T) {
	userID := uuid.New()

	tokenString, err := MakeJWT(userID, testSecret, testLifetime)
	if err != nil {
		t.Fatalf("MakeJWT returned unexpected error: %v", err)
	}
	if tokenString == "" {
		t.Fatal("MakeJWT returned an empty token")
	}

	// A JWT must have exactly three dot-separated parts.
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a JWT with 3 parts, got %d", len(parts))
	}

	// The token must be parseable and carry the expected claims.
	claims := &jwt.RegisteredClaims{}
	_, err = jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse generated token: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Errorf("expected Subject %q, got %q", userID, claims.Subject)
	}
	if claims.Issuer != "chirpy-access" {
		t.Errorf("expected Issuer %q, got %q", "chirpy-access", claims.Issuer)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	wantExpiry := time.Now().UTC().Add(testLifetime)
	if diff := claims.ExpiresAt.Time.Sub(wantExpiry); diff < -time.Minute || diff > time.Minute {
		t.Errorf("ExpiresAt %v is not close to expected %v", claims.ExpiresAt.Time, wantExpiry)
	}
}

func TestValidateJWT(t *testing.T) {
	userID := uuid.New()

	tokenString, err := MakeJWT(userID, testSecret, testLifetime)
	if err != nil {
		t.Fatalf("MakeJWT returned unexpected error: %v", err)
	}

	gotID, err := ValidateJWT(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateJWT returned unexpected error: %v", err)
	}
	if gotID != userID {
		t.Errorf("expected user ID %v, got %v", userID, gotID)
	}
}

func TestValidateJWTExpiredToken(t *testing.T) {
	userID := uuid.New()

	// A negative lifetime produces a token whose expiry is already in the past.
	tokenString, err := MakeJWT(userID, testSecret, -time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT returned unexpected error: %v", err)
	}

	gotID, err := ValidateJWT(tokenString, testSecret)
	if err == nil {
		t.Fatalf("expected expired token to be rejected, got user ID %v", gotID)
	}
	if gotID != uuid.Nil {
		t.Errorf("expected uuid.Nil on error, got %v", gotID)
	}
	if !errorsIsExpired(err) {
		t.Logf("error was not a recognized expiry error: %v", err)
	}
}

func TestValidateJWTWrongSecret(t *testing.T) {
	userID := uuid.New()

	// Token signed with testSecret must not validate against wrongSecret.
	tokenString, err := MakeJWT(userID, testSecret, testLifetime)
	if err != nil {
		t.Fatalf("MakeJWT returned unexpected error: %v", err)
	}

	gotID, err := ValidateJWT(tokenString, wrongSecret)
	if err == nil {
		t.Fatalf("expected token signed with wrong secret to be rejected, got user ID %v", gotID)
	}
	if gotID != uuid.Nil {
		t.Errorf("expected uuid.Nil on error, got %v", gotID)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "signature") {
		t.Errorf("expected a signature verification error, got: %v", err)
	}
}

func TestValidateJWTGarbageToken(t *testing.T) {
	_, err := ValidateJWT("this.is.not.a.jwt", testSecret)
	if err == nil {
		t.Fatal("expected garbage token to be rejected")
	}
}

func TestValidateJWTInvalidSubject(t *testing.T) {
	// Manually sign a well-formed token whose Subject is not a valid UUID.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(testLifetime)),
		Subject:   "not-a-uuid",
	})
	tokenString, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, err = ValidateJWT(tokenString, testSecret)
	if err == nil {
		t.Fatal("expected token with invalid Subject to be rejected")
	}
}

func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer abc123token")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("GetBearerToken returned unexpected error: %v", err)
	}
	if token != "abc123token" {
		t.Errorf("expected token %q, got %q", "abc123token", token)
	}
}

func TestGetBearerTokenStripsWhitespace(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer   spaced-token \t ")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("GetBearerToken returned unexpected error: %v", err)
	}
	if token != "spaced-token" {
		t.Errorf("expected token %q, got %q", "spaced-token", token)
	}
}

func TestGetBearerTokenMissingHeader(t *testing.T) {
	_, err := GetBearerToken(http.Header{})
	if err == nil {
		t.Fatal("expected an error when the Authorization header is missing")
	}
}

func TestGetBearerTokenMissingBearerPrefix(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Token abc123token")

	_, err := GetBearerToken(headers)
	if err == nil {
		t.Fatal("expected an error when the Bearer prefix is missing")
	}
}

func TestGetBearerTokenEmptyToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer ")

	_, err := GetBearerToken(headers)
	if err == nil {
		t.Fatal("expected an error when the token is empty")
	}
}

func TestGetAPIKey(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "ApiKey abc123key")

	key, err := GetAPIKey(headers)
	if err != nil {
		t.Fatalf("GetAPIKey returned unexpected error: %v", err)
	}
	if key != "abc123key" {
		t.Errorf("expected key %q, got %q", "abc123key", key)
	}
}

func TestGetAPIKeyStripsWhitespace(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "ApiKey   spaced-key \t ")

	key, err := GetAPIKey(headers)
	if err != nil {
		t.Fatalf("GetAPIKey returned unexpected error: %v", err)
	}
	if key != "spaced-key" {
		t.Errorf("expected key %q, got %q", "spaced-key", key)
	}
}

func TestGetAPIKeyMissingHeader(t *testing.T) {
	_, err := GetAPIKey(http.Header{})
	if err == nil {
		t.Fatal("expected an error when the Authorization header is missing")
	}
}

func TestGetAPIKeyMissingApiKeyPrefix(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer abc123key")

	_, err := GetAPIKey(headers)
	if err == nil {
		t.Fatal("expected an error when the ApiKey prefix is missing")
	}
}

func TestGetAPIKeyEmptyKey(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "ApiKey ")

	_, err := GetAPIKey(headers)
	if err == nil {
		t.Fatal("expected an error when the API key is empty")
	}
}

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("my-secret-password")
	if err != nil {
		t.Fatalf("HashPassword returned unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned an empty hash")
	}
	if hash == "my-secret-password" {
		t.Fatal("HashPassword returned the plaintext password")
	}

	// Argon2id hashes are salted, so two hashes of the same password differ.
	otherHash, err := HashPassword("my-secret-password")
	if err != nil {
		t.Fatalf("HashPassword returned unexpected error: %v", err)
	}
	if otherHash == hash {
		t.Error("expected different salts to produce different hashes")
	}
}

func TestCheckPassword(t *testing.T) {
	hash, err := HashPassword("my-secret-password")
	if err != nil {
		t.Fatalf("HashPassword returned unexpected error: %v", err)
	}

	match, err := CheckPassword("my-secret-password", hash)
	if err != nil {
		t.Fatalf("CheckPassword returned unexpected error: %v", err)
	}
	if !match {
		t.Error("expected correct password to match its hash")
	}

	match, err = CheckPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("CheckPassword returned unexpected error: %v", err)
	}
	if match {
		t.Error("expected wrong password to not match the hash")
	}
}

// errorsIsExpired reports whether err is caused by token expiry.
func errorsIsExpired(err error) bool {
	return errors.Is(err, jwt.ErrTokenExpired)
}
