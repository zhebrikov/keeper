package crypto_test

import (
	"testing"

	"github.com/zhebrikov/gophkeeper/internal/crypto"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := crypto.HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !crypto.CheckPassword(hash, "secret123") {
		t.Fatal("expected password to match")
	}
	if crypto.CheckPassword(hash, "wrong") {
		t.Fatal("expected password mismatch")
	}
}

func TestEncryptDecryptWithPassword(t *testing.T) {
	plaintext := []byte("super secret data")
	encrypted, err := crypto.EncryptWithPassword("master", plaintext)
	if err != nil {
		t.Fatalf("EncryptWithPassword: %v", err)
	}
	if len(encrypted) == 0 {
		t.Fatal("expected non-empty ciphertext")
	}

	decrypted, err := crypto.DecryptWithPassword("master", encrypted)
	if err != nil {
		t.Fatalf("DecryptWithPassword: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWithWrongPassword(t *testing.T) {
	encrypted, err := crypto.EncryptWithPassword("master", []byte("data"))
	if err != nil {
		t.Fatalf("EncryptWithPassword: %v", err)
	}
	_, err = crypto.DecryptWithPassword("wrong", encrypted)
	if err == nil {
		t.Fatal("expected decryption error")
	}
}

func TestHashToken(t *testing.T) {
	h1 := crypto.HashToken("token-a")
	h2 := crypto.HashToken("token-b")
	if h1 == h2 {
		t.Fatal("expected different hashes for different tokens")
	}
	if crypto.HashToken("token-a") != h1 {
		t.Fatal("expected deterministic hash")
	}
}

func TestDeriveKey(t *testing.T) {
	salt := []byte("1234567890123456")
	k1 := crypto.DeriveKey("password", salt)
	k2 := crypto.DeriveKey("password", salt)
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatal("expected deterministic key derivation")
		}
	}
}

func TestEncryptProducesValidBlob(t *testing.T) {
	key := crypto.DeriveKey("master", []byte("1234567890123456"))
	plaintext := []byte("secret payload")

	encrypted, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(encrypted) < 16+12+len(plaintext) {
		t.Fatalf("encrypted blob too short: %d bytes", len(encrypted))
	}

	encrypted2, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if string(encrypted) == string(encrypted2) {
		t.Fatal("expected different ciphertext due to random salt/nonce")
	}
}

func TestDecryptWithPasswordTooShort(t *testing.T) {
	_, err := crypto.DecryptWithPassword("master", []byte("short"))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestDecryptWithPasswordEmptyData(t *testing.T) {
	_, err := crypto.DecryptWithPassword("master", nil)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}
