// Package auth provides JWT token generation and validation.
//
// Access tokens are HS256-signed JWTs with the user ID in claims.
// Refresh tokens are opaque values stored separately in the database.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims holds JWT claims for an authenticated user.
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// Manager handles JWT access token operations.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager creates a JWT manager with the given secret and token TTL.
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// GenerateAccessToken creates a signed JWT for the given user ID.
func (m *Manager) GenerateAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ValidateAccessToken parses and validates a JWT, returning the user ID.
func (m *Manager) ValidateAccessToken(tokenStr string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token")
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse user id: %w", err)
	}
	return userID, nil
}
