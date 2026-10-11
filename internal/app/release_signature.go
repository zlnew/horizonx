// Release signature verification (minisign over SHA256SUMS).
//
// SHA256SUMS is published unsigned today, so release trust is TLS-only:
// whoever can serve or intercept the release can substitute both the
// tarball and its checksum. When HORIZONX_RELEASE_PUBKEY holds the minisign
// public key text (the single base64 line), every consumer must verify the
// SHA256SUMS.minisig signature BEFORE trusting the checksums — and fail
// closed on any problem. When the variable is unset or empty, behaviour is
// exactly today's: no signature is fetched or required.
package app

import (
	"fmt"
	"os"
	"strings"

	"aead.dev/minisign"
)

// envReleasePubkey configures the minisign public key text used to verify
// release SHA256SUMS signatures. Unset/empty = unsigned legacy behaviour.
const envReleasePubkey = "HORIZONX_RELEASE_PUBKEY"

// VerifyReleaseSums verifies the minisign signature sig over the SHA256SUMS
// bytes sums. It fails closed whenever a public key is configured: an
// unparseable key, a missing signature, or a bad signature all return an
// error. With no key configured it returns nil without checking anything,
// so existing installs keep working untouched.
//
// Callers must gate their checksum logic on this: do not extract or trust
// checksums from sums until VerifyReleaseSums has returned nil.
func VerifyReleaseSums(sums, sig []byte) error {
	keyText := strings.TrimSpace(os.Getenv(envReleasePubkey))
	if keyText == "" {
		return nil
	}
	var pub minisign.PublicKey
	if err := pub.UnmarshalText([]byte(keyText)); err != nil {
		return fmt.Errorf("bad %s: %v", envReleasePubkey, err)
	}
	if len(strings.TrimSpace(string(sig))) == 0 {
		return fmt.Errorf("release signature required (%s is set) but SHA256SUMS.minisig is missing", envReleasePubkey)
	}
	if !minisign.Verify(pub, sums, sig) {
		return fmt.Errorf("release signature verification failed for SHA256SUMS")
	}
	return nil
}
