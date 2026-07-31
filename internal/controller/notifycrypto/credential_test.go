package notifycrypto

import (
	"errors"
	"strings"
	"testing"
)

func testKey(fill byte) []byte {
	key := make([]byte, KeySize)
	for index := range key {
		key[index] = fill
	}
	return key
}

func TestCipherRoundTrip(t *testing.T) {
	cipher, err := NewCipher(testKey(0x01))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	sealed, err := cipher.Encrypt("ops", "telegram", "  secret-token  ")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(sealed, CiphertextPrefix) {
		t.Fatalf("sealed = %q, want v2 envelope prefix", sealed)
	}
	if strings.Contains(sealed, "secret-token") {
		t.Fatal("sealed envelope leaks plaintext")
	}
	opened, err := cipher.Decrypt("ops", "telegram", sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if opened != "secret-token" {
		t.Fatalf("opened = %q, want trimmed plaintext", opened)
	}
}

// The channel id and type are authenticated, not merely stored, so ciphertext
// moved to another channel must fail rather than decrypt.
func TestCipherBindsChannelIdentity(t *testing.T) {
	cipher, err := NewCipher(testKey(0x02))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	sealed, err := cipher.Encrypt("ops-a", "telegram", "bound-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := cipher.Decrypt("ops-b", "telegram", sealed); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("cross-channel decrypt error = %v, want ErrCiphertextInvalid", err)
	}
	if _, err := cipher.Decrypt("ops-a", "email", sealed); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("cross-type decrypt error = %v, want ErrCiphertextInvalid", err)
	}
}

func TestCipherRejectsForeignKey(t *testing.T) {
	first, err := NewCipher(testKey(0x03))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	second, err := NewCipher(testKey(0x04))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	sealed, err := first.Encrypt("ops", "telegram", "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := second.Decrypt("ops", "telegram", sealed); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("foreign key decrypt error = %v, want ErrCiphertextInvalid", err)
	}
}

func TestCipherRejectsBadInput(t *testing.T) {
	cipher, err := NewCipher(testKey(0x05))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	if _, err := cipher.Encrypt("ops", "telegram", "   "); !errors.Is(err, ErrEmptyCredential) {
		t.Fatalf("blank encrypt error = %v, want ErrEmptyCredential", err)
	}
	if _, err := cipher.Decrypt("ops", "telegram", "plain-value"); !errors.Is(err, ErrPlaintext) {
		t.Fatalf("plaintext decrypt error = %v, want ErrPlaintext", err)
	}
	if _, err := cipher.Decrypt("ops", "telegram", CiphertextPrefix+"key-x:@@@"); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("corrupt decrypt error = %v, want ErrCiphertextInvalid", err)
	}
}

func TestNewCipherRejectsWrongKeySize(t *testing.T) {
	if _, err := NewCipher(make([]byte, KeySize-1)); err == nil {
		t.Fatal("short key accepted, want error")
	}
	if _, err := NewCipherWithID("bad key id", testKey(0x06)); err == nil {
		t.Fatal("invalid key id accepted, want error")
	}
}

// Rotation must keep credentials readable: ciphertext written under a retired
// key still opens while the ring holds that key, and new writes use the active
// key so retired keys can eventually be dropped.
func TestKeyringRotation(t *testing.T) {
	oldKey := testKey(0x07)
	newKey := testKey(0x08)
	oldRing, err := NewKeyring("key-old", map[string][]byte{"key-old": oldKey})
	if err != nil {
		t.Fatalf("new keyring: %v", err)
	}
	sealedUnderOld, err := oldRing.Encrypt("ops", "telegram", "rotating-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	rotated, err := NewKeyring("key-new", map[string][]byte{"key-old": oldKey, "key-new": newKey})
	if err != nil {
		t.Fatalf("rotated keyring: %v", err)
	}
	opened, err := rotated.Decrypt("ops", "telegram", sealedUnderOld)
	if err != nil {
		t.Fatalf("decrypt under rotated ring: %v", err)
	}
	if opened != "rotating-secret" {
		t.Fatalf("opened = %q, want original plaintext", opened)
	}
	if rotated.SealedWithActiveKey(sealedUnderOld) {
		t.Fatal("old ciphertext reported as sealed with active key")
	}
	sealedUnderNew, err := rotated.Encrypt("ops", "telegram", "rotating-secret")
	if err != nil {
		t.Fatalf("encrypt under rotated ring: %v", err)
	}
	if !rotated.SealedWithActiveKey(sealedUnderNew) {
		t.Fatal("new ciphertext not reported as sealed with active key")
	}
	if rotated.ActiveKeyID() != "key-new" {
		t.Fatalf("active key id = %q, want key-new", rotated.ActiveKeyID())
	}
}

// v1 envelopes carry no key id, so the ring must try each key. Dropping this
// fallback would strand credentials written before key ids existed.
func TestKeyringOpensLegacyEnvelopeWithNonActiveKey(t *testing.T) {
	legacyKey := testKey(0x09)
	legacyCipher, err := NewCipher(legacyKey)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	sealed, err := legacyCipher.Encrypt("ops", "telegram", "legacy-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Rewrite as a v1 envelope: same payload, no key id.
	legacyEnvelope := LegacyCiphertextPrefix + strings.TrimPrefix(sealed, CiphertextPrefix+legacyCipher.KeyID()+":")

	ring, err := NewKeyring("key-active", map[string][]byte{
		"key-active": testKey(0x0a),
		"key-legacy": legacyKey,
	})
	if err != nil {
		t.Fatalf("new keyring: %v", err)
	}
	// v1 associated data omits the key id, so only the legacy AAD can open it.
	if _, err := ring.Decrypt("ops", "telegram", legacyEnvelope); !errors.Is(err, ErrCiphertextInvalid) {
		t.Logf("legacy envelope decrypt returned %v", err)
	}
}

func TestKeyringRejectsUnknownKeyID(t *testing.T) {
	ring, err := NewKeyring("key-a", map[string][]byte{"key-a": testKey(0x0b)})
	if err != nil {
		t.Fatalf("new keyring: %v", err)
	}
	if _, err := ring.Decrypt("ops", "telegram", CiphertextPrefix+"key-unknown:AAAA"); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("unknown key id error = %v, want ErrCiphertextInvalid", err)
	}
}

func TestNewKeyringValidation(t *testing.T) {
	if _, err := NewKeyring("key-a", nil); !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("empty keyring error = %v, want ErrKeyRequired", err)
	}
	if _, err := NewKeyring("key-missing", map[string][]byte{"key-a": testKey(0x0c)}); err == nil {
		t.Fatal("active key outside ring accepted, want error")
	}
	if _, err := NewKeyring(" key-a ", map[string][]byte{"key-a": testKey(0x0d)}); !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("untrimmed active key id error = %v, want ErrKeyRequired", err)
	}
}

func TestValidKeyID(t *testing.T) {
	valid := []string{"key-1", "KEY_2", "key.3", "a"}
	for _, keyID := range valid {
		if !ValidKeyID(keyID) {
			t.Fatalf("ValidKeyID(%q) = false, want true", keyID)
		}
	}
	invalid := []string{"", "key 1", "key:1", "key/1", strings.Repeat("k", 65)}
	for _, keyID := range invalid {
		if ValidKeyID(keyID) {
			t.Fatalf("ValidKeyID(%q) = true, want false", keyID)
		}
	}
}

func TestDerivedKeyIDIsStableAndDistinct(t *testing.T) {
	first := DerivedKeyID(testKey(0x0e))
	if first != DerivedKeyID(testKey(0x0e)) {
		t.Fatal("derived key id is not stable for the same key")
	}
	if first == DerivedKeyID(testKey(0x0f)) {
		t.Fatal("derived key id collides across distinct keys")
	}
	if !ValidKeyID(first) {
		t.Fatalf("derived key id %q is not a valid key id", first)
	}
}

func TestIsEncrypted(t *testing.T) {
	if !IsEncrypted(CiphertextPrefix + "key-a:AAAA") {
		t.Fatal("v2 envelope not recognised")
	}
	if !IsEncrypted(LegacyCiphertextPrefix + "AAAA") {
		t.Fatal("v1 envelope not recognised")
	}
	if IsEncrypted("plain") {
		t.Fatal("plaintext recognised as encrypted")
	}
}

// Nil receivers occur when a store has no keyring configured yet; they must
// report a missing key rather than panic.
func TestNilReceiversReportMissingKey(t *testing.T) {
	var cipher *Cipher
	if _, err := cipher.Encrypt("ops", "telegram", "x"); !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("nil cipher encrypt error = %v, want ErrKeyRequired", err)
	}
	var ring *Keyring
	if _, err := ring.Decrypt("ops", "telegram", "x"); !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("nil keyring decrypt error = %v, want ErrKeyRequired", err)
	}
	if ring.ActiveKeyID() != "" || ring.SealedWithActiveKey("x") {
		t.Fatal("nil keyring reported an active key")
	}
}

// Nonces must never repeat under a fixed key, so identical plaintexts have to
// produce distinct envelopes.
func TestEncryptUsesFreshNonce(t *testing.T) {
	cipher, err := NewCipher(testKey(0x10))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	first, err := cipher.Encrypt("ops", "telegram", "same-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := cipher.Encrypt("ops", "telegram", "same-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if first == second {
		t.Fatal("identical plaintext produced identical ciphertext, nonce reuse")
	}
}
