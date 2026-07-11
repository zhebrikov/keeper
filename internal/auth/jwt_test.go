package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhebrikov/gophkeeper/internal/auth"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	manager := auth.NewManager("test-secret", time.Minute)
	userID := uuid.New()

	token, err := manager.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	parsed, err := manager.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if parsed != userID {
		t.Fatalf("got %v, want %v", parsed, userID)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	manager := auth.NewManager("test-secret", time.Minute)
	_, err := manager.ValidateAccessToken("invalid.token.here")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	m1 := auth.NewManager("secret-a", time.Minute)
	m2 := auth.NewManager("secret-b", time.Minute)
	userID := uuid.New()

	token, err := m1.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	_, err = m2.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected validation error for wrong secret")
	}
}
