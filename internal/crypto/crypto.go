// Package crypto provides encryption, hashing, and key derivation utilities.
//
// Passwords for server accounts are hashed with bcrypt ([HashPassword]).
// Refresh tokens are stored as SHA-256 hashes ([HashToken]).
// Secret payloads are encrypted on the client with AES-GCM; the master password
// is stretched via PBKDF2 ([EncryptWithPassword], [DecryptWithPassword]).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

const (
	bcryptCost    = 12
	pbkdf2Iter    = 100_000
	pbkdf2KeyLen  = 32
	pbkdf2SaltLen = 16
)

// HashPassword returns a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the stored bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// DeriveKey derives a 256-bit AES key from a master password and salt using PBKDF2.
func DeriveKey(masterPassword string, salt []byte) []byte {
	return pbkdf2.Key([]byte(masterPassword), salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
}

// Encrypt encrypts plaintext with AES-GCM using a pre-derived key.
// The returned blob contains salt || nonce || ciphertext.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

// EncryptWithPassword encrypts plaintext using a master password.
// The returned blob contains salt || nonce || ciphertext; salt is used with
// PBKDF2 to derive the AES-256 key.
func EncryptWithPassword(masterPassword string, plaintext []byte) ([]byte, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	key := DeriveKey(masterPassword, salt)
	encrypted, err := encryptWithKey(key, plaintext)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, len(salt)+len(encrypted))
	result = append(result, salt...)
	result = append(result, encrypted...)
	return result, nil
}

// DecryptWithPassword decrypts data produced by [EncryptWithPassword].
func DecryptWithPassword(masterPassword string, data []byte) ([]byte, error) {
	if len(data) < pbkdf2SaltLen+1 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	salt := data[:pbkdf2SaltLen]
	key := DeriveKey(masterPassword, salt)
	return decryptWithKey(key, data[pbkdf2SaltLen:])
}

func encryptWithKey(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptWithKey(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// HashToken returns a SHA-256 hash of a token encoded as base64.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(sum[:])
}
