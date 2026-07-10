package models_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
)

func TestSecretIsDeleted(t *testing.T) {
	secret := &models.Secret{}
	if secret.IsDeleted() {
		t.Fatal("expected secret to be active")
	}

	now := time.Now()
	secret.DeletedAt = &now
	if !secret.IsDeleted() {
		t.Fatal("expected secret to be deleted")
	}
}

func TestSecretTypes(t *testing.T) {
	types := []models.SecretType{
		models.SecretTypeCredential,
		models.SecretTypeText,
		models.SecretTypeBinary,
		models.SecretTypeCard,
		models.SecretTypeOTP,
	}
	for _, st := range types {
		if st == "" {
			t.Fatal("secret type should not be empty")
		}
	}
}

func TestUserFields(t *testing.T) {
	user := models.User{
		ID:    uuid.New(),
		Login: "alice",
	}
	if user.Login != "alice" {
		t.Fatalf("unexpected login: %s", user.Login)
	}
}
