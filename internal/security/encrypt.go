// Package security provides encryption-at-rest helpers for sensitive values
// (P1-11). AES-256-GCM with a random 12-byte nonce prepended to the ciphertext.
// Keys are derived via KeyFromSecret from dedicated config (ENV_ENCRYPTION_KEY)
// or fallback JWT_SECRET.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// KeyFromSecret derives a 32-byte AES-256 key from any secret string via
// SHA-256. Used for environment-variable encryption keys (from ENV_ENCRYPTION_KEY
// or legacy JWT_SECRET).
func KeyFromSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Encrypt returns base64(nonce || ciphertext) for the given plaintext.
// A random nonce per call means equal plaintexts never share ciphertext.
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to read nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. Returns an error on tampered data or a wrong key.
func Decrypt(encoded string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt (wrong key or tampered data): %w", err)
	}

	return string(plaintext), nil
}

// DecryptWithFallback attempts to decrypt encoded with primary key first.
// If primary fails and legacy is provided (len == 32), it attempts legacy decryption.
// When legacy decryption succeeds, usedLegacy is returned as true.
//
// Authenticated encryption (AES-GCM) guarantees that a successful legacy decryption
// is proof the ciphertext was encrypted under the legacy key; it cannot be produced
// by corruption or accidental collisions.
func DecryptWithFallback(encoded string, primary, legacy []byte) (plaintext string, usedLegacy bool, err error) {
	plaintext, err = Decrypt(encoded, primary)
	if err == nil {
		return plaintext, false, nil
	}

	if len(legacy) == 0 {
		return "", false, fmt.Errorf("primary decryption failed: %w", err)
	}

	legacyPlaintext, legacyErr := Decrypt(encoded, legacy)
	if legacyErr == nil {
		return legacyPlaintext, true, nil
	}

	return "", false, fmt.Errorf("decryption failed with primary (%v) and legacy (%v)", err, legacyErr)
}
