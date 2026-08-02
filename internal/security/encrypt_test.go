package security

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := KeyFromSecret("a-test-secret")
	plaintext := "SUPER_SECRET_API_KEY=abc123"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("round trip mismatch: %q != %q", decrypted, plaintext)
	}
}

func TestEncryptProducesUniqueCiphertext(t *testing.T) {
	key := KeyFromSecret("another-secret")
	a, _ := Encrypt("same-value", key)
	b, _ := Encrypt("same-value", key)

	if a == b {
		t.Fatal("equal plaintexts must not produce equal ciphertext (random nonce)")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	ciphertext, err := Encrypt("secret-value", KeyFromSecret("key-a"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if _, err := Decrypt(ciphertext, KeyFromSecret("key-b")); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	key := KeyFromSecret("key-a")
	ciphertext, err := Encrypt("secret-value", key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Flip a char in the base64 body.
	tampered := "A" + ciphertext[1:]

	if _, err := Decrypt(tampered, key); err == nil {
		t.Fatal("tampered ciphertext must fail auth")
	}
}

func TestKeyFromSecretAlways32Bytes(t *testing.T) {
	for _, secret := range []string{"", "short", strings.Repeat("x", 100)} {
		key := KeyFromSecret(secret)
		if len(key) != 32 {
			t.Fatalf("key from %q must be 32 bytes, got %d", secret, len(key))
		}
	}
}
