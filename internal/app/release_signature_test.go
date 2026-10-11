package app

import (
	"bytes"
	"testing"

	"aead.dev/minisign"
)

// signFixture generates a keypair in-process, signs sums, and returns the
// public key text (what HORIZONX_RELEASE_PUBKEY holds) plus the raw
// signature file bytes. Hermetic: no minisign binary involved.
func signFixture(t *testing.T, sums []byte) (pubkeyText string, sigBytes []byte) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub.String(), minisign.Sign(priv, sums)
}

func TestVerifyReleaseSumsValid(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	pub, sig := signFixture(t, sums)
	t.Setenv(envReleasePubkey, pub)
	if err := VerifyReleaseSums(sums, sig); err != nil {
		t.Fatalf("VerifyReleaseSums valid: %v", err)
	}
}

func TestVerifyReleaseSumsTamperedSums(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	pub, sig := signFixture(t, sums)
	t.Setenv(envReleasePubkey, pub)
	if err := VerifyReleaseSums([]byte("zzz999  horizonx-linux-x86_64.tar.gz\n"), sig); err == nil {
		t.Fatal("expected error for tampered sums, got nil")
	}
}

func TestVerifyReleaseSumsTamperedSig(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	pub, sig := signFixture(t, sums)
	t.Setenv(envReleasePubkey, pub)
	bad := bytes.Clone(sig)
	bad[len(bad)-1] ^= 0x01
	if err := VerifyReleaseSums(sums, bad); err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
}

func TestVerifyReleaseSumsMissingSig(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	pub, _ := signFixture(t, sums)
	t.Setenv(envReleasePubkey, pub)
	if err := VerifyReleaseSums(sums, nil); err == nil {
		t.Fatal("expected error for missing signature, got nil")
	}
}

func TestVerifyReleaseSumsKeyUnset(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	// Env unset (not merely empty): today's behaviour, unsigned sums pass.
	if err := VerifyReleaseSums(sums, nil); err != nil {
		t.Fatalf("key unset must preserve unsigned behaviour: %v", err)
	}
}

func TestVerifyReleaseSumsBadKey(t *testing.T) {
	sums := []byte("abc123  horizonx-linux-x86_64.tar.gz\n")
	_, sig := signFixture(t, sums)
	t.Setenv(envReleasePubkey, "not-a-valid-key!!!")
	if err := VerifyReleaseSums(sums, sig); err == nil {
		t.Fatal("expected error for unparseable key, got nil")
	}
}
