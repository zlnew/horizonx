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

func TestDecryptWithFallback(t *testing.T) {
	primaryKey := KeyFromSecret("primary-secret")
	legacyKey := KeyFromSecret("legacy-secret")
	thirdKey := KeyFromSecret("third-unrelated-secret")
	plaintext := "DATABASE_PASSWORD=super-secret-pw"

	// 1. Encrypted with primary key -> usedLegacy == false, correct plaintext
	t.Run("encrypted with primary key", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, primaryKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		got, usedLegacy, err := DecryptWithFallback(ciphertext, primaryKey, legacyKey)
		if err != nil {
			t.Fatalf("expected successful decrypt, got: %v", err)
		}
		if usedLegacy {
			t.Errorf("expected usedLegacy == false, got true")
		}
		if got != plaintext {
			t.Errorf("got plaintext %q, want %q", got, plaintext)
		}
	})

	// 2. Encrypted with legacy key -> usedLegacy == true, correct plaintext
	t.Run("encrypted with legacy key", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, legacyKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		got, usedLegacy, err := DecryptWithFallback(ciphertext, primaryKey, legacyKey)
		if err != nil {
			t.Fatalf("expected successful decrypt via legacy key, got: %v", err)
		}
		if !usedLegacy {
			t.Errorf("expected usedLegacy == true, got false")
		}
		if got != plaintext {
			t.Errorf("got plaintext %q, want %q", got, plaintext)
		}
	})

	// 3. Tampered ciphertext -> error
	t.Run("tampered ciphertext", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, primaryKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		tampered := "A" + ciphertext[1:]
		_, _, err = DecryptWithFallback(tampered, primaryKey, legacyKey)
		if err == nil {
			t.Fatal("expected error on tampered ciphertext, got nil")
		}
	})

	// 4. Encrypted with third unrelated key -> error, even when legacy is supplied
	t.Run("encrypted with third unrelated key", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, thirdKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		_, _, err = DecryptWithFallback(ciphertext, primaryKey, legacyKey)
		if err == nil {
			t.Fatal("expected error with unrelated key, got nil")
		}
	})

	// 5. No legacy key (nil/empty) with legacy-encrypted value -> error, not false success
	t.Run("no legacy key with legacy-encrypted value", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, legacyKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		// test with nil
		_, _, err = DecryptWithFallback(ciphertext, primaryKey, nil)
		if err == nil {
			t.Fatal("expected error when legacy is nil, got nil")
		}

		// test with empty
		_, _, err = DecryptWithFallback(ciphertext, primaryKey, []byte{})
		if err == nil {
			t.Fatal("expected error when legacy is empty, got nil")
		}
	})

	// 6. Wrong-length keys behave as Encrypt/Decrypt already define
	t.Run("wrong length primary key", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, primaryKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		_, _, err = DecryptWithFallback(ciphertext, []byte("short"), legacyKey)
		if err == nil {
			t.Fatal("expected error when primary key is not 32 bytes, got nil")
		}
	})

	t.Run("wrong length legacy key with legacy-encrypted ciphertext", func(t *testing.T) {
		ciphertext, err := Encrypt(plaintext, legacyKey)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		// Primary will fail to decrypt, legacy has invalid length
		_, _, err = DecryptWithFallback(ciphertext, primaryKey, []byte("short"))
		if err == nil {
			t.Fatal("expected error when legacy key is not 32 bytes, got nil")
		}
	})
}
