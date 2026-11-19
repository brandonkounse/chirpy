package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func mustMakeJWT(t *testing.T, id uuid.UUID, secret string, expiresIn time.Duration) string {
	t.Helper()
	token, err := MakeJWT(id, secret, expiresIn)
	if err != nil {
		t.Fatalf("setup: MakeJWT returned an error: %v", err)
	}
	return token
}

func TestMakeJWT(t *testing.T) {
	tests := []struct {
		name      string
		userID    uuid.UUID
		secret    string
		expiresIn time.Duration
	}{
		{"24 hour token", uuid.MustParse("14d8f0fb-4ff0-4f2a-8d9d-d467961aac9d"), "123456abcdef", 24 * time.Hour},
		{"1 minute token", uuid.MustParse("4ed5c171-2c5a-4d1b-aeed-8f179aa6e2eb"), "654321fedcba", time.Minute},
		{"1 hour token", uuid.MustParse("f1ea1f0b-14a4-451a-9203-0d55fc609756"), "abcdef123456", time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenString, err := MakeJWT(tc.userID, tc.secret, tc.expiresIn)
			if err != nil {
				t.Fatalf("MakeJWT returned an error: %v", err)
			}
			if tokenString == "" {
				t.Fatal("MakeJWT returned an empty token string")
			}

			claims := &jwt.RegisteredClaims{}
			_, err = jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (interface{}, error) {
				return []byte(tc.secret), nil
			})
			if err != nil {
				t.Fatalf("token didn't parse with the secret it was signed with: %v", err)
			}

			if claims.Subject != tc.userID.String() {
				t.Errorf("Subject = %q, want %q", claims.Subject, tc.userID.String())
			}
			if claims.Issuer != "chirpy-access" {
				t.Errorf("Issuer = %q, want %q", claims.Issuer, "chirpy-access")
			}

			const tolerance = 2 * time.Second

			if claims.ExpiresAt == nil {
				t.Fatal("ExpiresAt is not set")
			}
			wantExpiry := time.Now().Add(tc.expiresIn)
			if diff := claims.ExpiresAt.Time.Sub(wantExpiry); diff < -tolerance || diff > tolerance {
				t.Errorf("ExpiresAt = %v, want about %v", claims.ExpiresAt.Time, wantExpiry)
			}

			if claims.IssuedAt == nil {
				t.Fatal("IssuedAt is not set")
			}
			if diff := time.Since(claims.IssuedAt.Time); diff < -tolerance || diff > tolerance {
				t.Errorf("IssuedAt = %v, want about now", claims.IssuedAt.Time)
			}
		})
	}
}

func TestValidateJWT(t *testing.T) {
	userID := uuid.MustParse("14d8f0fb-4ff0-4f2a-8d9d-d467961aac9d")
	otherID := uuid.MustParse("4ed5c171-2c5a-4d1b-aeed-8f179aa6e2eb")
	secret := "123456abcdef"

	validToken := mustMakeJWT(t, userID, secret, time.Hour)
	expiredToken := mustMakeJWT(t, userID, secret, -time.Hour)

	partsA := strings.Split(validToken, ".")
	partsB := strings.Split(mustMakeJWT(t, otherID, secret, time.Hour), ".")
	forgedToken := partsA[0] + "." + partsB[1] + "." + partsA[2]

	signWithSubject := func(subject string) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		})
		s, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("setup: signing token failed: %v", err)
		}
		return s
	}

	tests := []struct {
		name    string
		token   string
		secret  string
		wantID  uuid.UUID
		wantErr bool
	}{
		{"valid token", validToken, secret, userID, false},
		{"wrong secret", validToken, "not-the-secret", uuid.Nil, true},
		{"expired token", expiredToken, secret, uuid.Nil, true},
		{"empty string", "", secret, uuid.Nil, true},
		{"garbage string", "this-is-not-a-jwt", secret, uuid.Nil, true},
		{"payload swapped after signing", forgedToken, secret, uuid.Nil, true},
		{"subject is not a UUID", signWithSubject("not-a-uuid"), secret, uuid.Nil, true},
		{"subject is missing", signWithSubject(""), secret, uuid.Nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateJWT(tc.token, tc.secret)

			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tc.wantErr)
			}

			if got != tc.wantID {
				t.Errorf("got ID %v, want %v", got, tc.wantID)
			}
		})
	}
}
