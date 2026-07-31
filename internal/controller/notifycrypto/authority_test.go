package notifycrypto

import (
	"errors"
	"strings"
	"testing"
)

// An absent authority is not an error: deployments without notification
// delivery must start normally and simply report themselves unauthorized.
func TestNewAuthorityKeyringUnconfigured(t *testing.T) {
	keyring, configured, err := NewAuthorityKeyring("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configured {
		t.Fatal("empty input must report unconfigured")
	}
	if keyring != nil {
		t.Fatal("unconfigured input must not produce a keyring")
	}
}

// A declared active key with no ring is a misconfiguration, not an absent
// authority; it must not silently fall through to "unconfigured".
func TestNewAuthorityKeyringActiveWithoutRing(t *testing.T) {
	if _, _, err := NewAuthorityKeyring("authority-0123456789abcdef", nil); !errors.Is(err, ErrAuthorityKeyring) {
		t.Fatalf("want ErrAuthorityKeyring, got %v", err)
	}
}

// Untrimmed ids are rejected rather than trimmed. Accepting them would let
// " k" alias the stored binding for "k" and hand authority to a different key.
func TestNewAuthorityKeyringRejectsUntrimmedIdentifiers(t *testing.T) {
	valid := "authority-0123456789abcdef"
	cases := []struct {
		name   string
		active string
		keys   map[string]string
	}{
		{"untrimmed active", " " + valid, map[string]string{valid: "secret"}},
		{"untrimmed ring id", valid, map[string]string{" " + valid: "secret"}},
		{"invalid active id", "Authority Key!", map[string]string{"Authority Key!": "secret"}},
		{"blank key", valid, map[string]string{valid: "   "}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, err := NewAuthorityKeyring(testCase.active, testCase.keys); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

// Binding to an active key absent from the ring would persist a fingerprint the
// controller cannot reproduce, permanently locking itself out of the database.
func TestNewAuthorityKeyringActiveKeyMustBePresent(t *testing.T) {
	_, _, err := NewAuthorityKeyring("authority-0000000000000000", map[string]string{
		"authority-1111111111111111": "secret",
	})
	if !errors.Is(err, ErrAuthorityActiveKeyMissing) {
		t.Fatalf("want ErrAuthorityActiveKeyMissing, got %v", err)
	}
}

// Matches accepts any key in the ring, which is what allows a rotation to
// prove continuity with the previously stored fingerprint before advancing it.
func TestAuthorityKeyringMatchesAnyRingMember(t *testing.T) {
	oldKey := "old-secret"
	newKey := "new-secret"
	keyring, configured, err := NewAuthorityKeyring("authority-new00000000000", map[string]string{
		"authority-new00000000000": newKey,
		"authority-old00000000000": oldKey,
	})
	if err != nil || !configured {
		t.Fatalf("setup failed: configured=%v err=%v", configured, err)
	}
	if got := keyring.ActiveFingerprint(); got != AuthorityFingerprint(newKey) {
		t.Fatalf("active fingerprint mismatch: %s", got)
	}
	if !keyring.Matches(AuthorityFingerprint(oldKey)) {
		t.Fatal("rotation must match the superseded key")
	}
	if !keyring.Matches(AuthorityFingerprint(newKey)) {
		t.Fatal("must match its own active key")
	}
	if keyring.Matches(AuthorityFingerprint("unrelated")) {
		t.Fatal("unrelated key must not match")
	}
	if keyring.Matches("") {
		t.Fatal("empty stored fingerprint must not match")
	}
}

// Keys are trimmed before hashing, so whitespace introduced by a secrets file
// or editor cannot change the derived binding.
func TestAuthorityFingerprintTrimsKey(t *testing.T) {
	if AuthorityFingerprint(" secret\n") != AuthorityFingerprint("secret") {
		t.Fatal("fingerprint must ignore surrounding whitespace")
	}
	if len(AuthorityFingerprint("secret")) != 64 {
		t.Fatal("fingerprint must be a full hex sha256")
	}
}

// The single-key API must derive the same id an explicit one-key ring uses, so
// upgrading from the legacy call does not rebind the database.
func TestAuthorityDerivedKeyIDIsStableAndValid(t *testing.T) {
	derived := AuthorityDerivedKeyID("secret")
	if !strings.HasPrefix(derived, "authority-") {
		t.Fatalf("unexpected prefix: %s", derived)
	}
	if len(derived) != len("authority-")+16 {
		t.Fatalf("unexpected length: %s", derived)
	}
	if !ValidKeyID(derived) {
		t.Fatalf("derived id must satisfy ValidKeyID: %s", derived)
	}
	if derived != AuthorityDerivedKeyID(" secret ") {
		t.Fatal("derived id must be whitespace-insensitive")
	}
}

// Fingerprints must not be recoverable to the key, and must not embed it.
func TestAuthorityFingerprintDoesNotLeakKey(t *testing.T) {
	key := "super-secret-token"
	if strings.Contains(AuthorityFingerprint(key), key) {
		t.Fatal("fingerprint must not contain the key")
	}
	if strings.Contains(AuthorityDerivedKeyID(key), key) {
		t.Fatal("derived key id must not contain the key")
	}
}

// A nil keyring must be inert rather than panicking, since callers hold it as
// a pointer that is nil whenever authority is unconfigured.
func TestAuthorityKeyringNilReceiver(t *testing.T) {
	var keyring *AuthorityKeyring
	if keyring.ActiveKeyID() != "" || keyring.ActiveFingerprint() != "" {
		t.Fatal("nil keyring must expose empty values")
	}
	if keyring.Matches("anything") {
		t.Fatal("nil keyring must not match")
	}
}
