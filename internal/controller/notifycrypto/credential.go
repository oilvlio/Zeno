// Package notifycrypto implements authenticated encryption for stored
// notification channel credentials.
//
// The package is deliberately free of storage and HTTP concerns so credential
// handling can be reviewed and tested as one self-contained security unit. The
// ciphertext envelope, associated data layout and key-id derivation are part of
// the on-disk format: changing them invalidates credentials already stored by
// deployed controllers.
package notifycrypto

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	// KeySize is the required raw key length in bytes.
	KeySize = 32
	// LegacyCiphertextPrefix marks v1 envelopes, which carried no key id.
	LegacyCiphertextPrefix = "zeno:notification-credential:v1:aes-256-gcm:"
	// CiphertextPrefix marks the current key-id addressable envelope.
	CiphertextPrefix = "zeno:notification-credential:v2:aes-256-gcm:"

	aadDomain          = "zeno.notification.credential\x00v1\x00"
	maximumKeyIDLength = 64
	defaultChannelType = "telegram"
)

var (
	// ErrKeyRequired reports a missing or unusable credential key.
	ErrKeyRequired = errors.New("notification credential key required")
	// ErrCiphertextInvalid reports an envelope that failed to authenticate.
	ErrCiphertextInvalid = errors.New("invalid notification credential ciphertext")
	// ErrPlaintext reports a stored credential that was never encrypted.
	ErrPlaintext = errors.New("unencrypted notification credential")
	// ErrEmptyCredential reports a blank credential. Callers map this onto
	// their own request-validation error.
	ErrEmptyCredential = errors.New("empty notification credential")
)

// Cipher encrypts and decrypts credentials under a single key.
type Cipher struct {
	keyID string
	aead  cipher.AEAD
}

// Keyring resolves ciphertext to the key that produced it, so keys can be
// rotated without asking administrators to re-enter every credential.
type Keyring struct {
	activeKeyID string
	active      *Cipher
	byID        map[string]*Cipher
	legacyOrder []*Cipher
}

// NewCipher builds a cipher whose key id is derived from the key itself.
func NewCipher(key []byte) (*Cipher, error) {
	return NewCipherWithID(DerivedKeyID(key), key)
}

// NewCipherWithID builds a cipher bound to an explicit key id.
func NewCipherWithID(keyID string, key []byte) (*Cipher, error) {
	normalizedKeyID := strings.TrimSpace(keyID)
	if normalizedKeyID != keyID || !ValidKeyID(normalizedKeyID) {
		return nil, fmt.Errorf("invalid notification credential key id")
	}
	keyID = normalizedKeyID
	if len(key) != KeySize {
		return nil, fmt.Errorf("notification credential key must be %d bytes", KeySize)
	}
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	block, err := aes.NewCipher(keyCopy)
	zeroBytes(keyCopy)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{keyID: keyID, aead: aead}, nil
}

// NewKeyring installs a key-id addressable ring with one active key.
func NewKeyring(activeKeyID string, keys map[string][]byte) (*Keyring, error) {
	normalizedActiveKeyID := strings.TrimSpace(activeKeyID)
	if normalizedActiveKeyID != activeKeyID || !ValidKeyID(normalizedActiveKeyID) || len(keys) == 0 {
		return nil, ErrKeyRequired
	}
	normalizedKeys := make(map[string][]byte, len(keys))
	keyIDs := make([]string, 0, len(keys))
	for rawKeyID, key := range keys {
		keyID := strings.TrimSpace(rawKeyID)
		if keyID != rawKeyID || !ValidKeyID(keyID) {
			return nil, fmt.Errorf("invalid notification credential key id")
		}
		if _, duplicate := normalizedKeys[keyID]; duplicate {
			return nil, fmt.Errorf("duplicate notification credential key id")
		}
		normalizedKeys[keyID] = key
		keyIDs = append(keyIDs, keyID)
	}
	sort.Strings(keyIDs)
	ring := &Keyring{
		activeKeyID: normalizedActiveKeyID,
		byID:        make(map[string]*Cipher, len(keys)),
	}
	for _, keyID := range keyIDs {
		keyCipher, err := NewCipherWithID(keyID, normalizedKeys[keyID])
		if err != nil {
			return nil, err
		}
		ring.byID[keyID] = keyCipher
	}
	ring.active = ring.byID[normalizedActiveKeyID]
	if ring.active == nil {
		return nil, fmt.Errorf("active notification credential key id is not in key ring")
	}
	// Legacy v1 ciphertext had no key id. Try the active key first, then the
	// remaining keys in deterministic order during a rolling rotation.
	ring.legacyOrder = append(ring.legacyOrder, ring.active)
	for _, keyID := range keyIDs {
		if keyID != normalizedActiveKeyID {
			ring.legacyOrder = append(ring.legacyOrder, ring.byID[keyID])
		}
	}
	return ring, nil
}

// KeyID reports the key id embedded in envelopes this cipher produces.
func (c *Cipher) KeyID() string {
	if c == nil {
		return ""
	}
	return c.keyID
}

// Encrypt seals a credential, binding it to the owning channel and type.
func (c *Cipher) Encrypt(channelID, channelType, credential string) (string, error) {
	if c == nil || c.aead == nil || !ValidKeyID(c.keyID) {
		return "", ErrKeyRequired
	}
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return "", ErrEmptyCredential
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return "", err
	}
	plaintext := []byte(credential)
	sealed := c.aead.Seal(nil, nonce, plaintext, associatedDataV2(channelID, channelType, c.keyID))
	zeroBytes(plaintext)
	payload := make([]byte, 0, len(nonce)+len(sealed))
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	zeroBytes(payload)
	return CiphertextPrefix + c.keyID + ":" + encoded, nil
}

// Decrypt opens an envelope produced for the same channel and type.
func (c *Cipher) Decrypt(channelID, channelType, storedCredential string) (string, error) {
	if c == nil || c.aead == nil {
		return "", ErrKeyRequired
	}
	storedCredential = strings.TrimSpace(storedCredential)
	if storedCredential == "" {
		return "", ErrEmptyCredential
	}
	if strings.HasPrefix(storedCredential, CiphertextPrefix) {
		keyID, encoded, ok := parseV2Envelope(storedCredential)
		if !ok || keyID != c.keyID {
			return "", ErrCiphertextInvalid
		}
		return c.decryptPayload(channelID, channelType, keyID, encoded, true)
	}
	if strings.HasPrefix(storedCredential, LegacyCiphertextPrefix) {
		encoded := strings.TrimPrefix(storedCredential, LegacyCiphertextPrefix)
		return c.decryptPayload(channelID, channelType, "", encoded, false)
	}
	return "", ErrPlaintext
}

func (c *Cipher) decryptPayload(channelID, channelType, keyID, encoded string, v2 bool) (string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrCiphertextInvalid
	}
	defer zeroBytes(payload)
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize+c.aead.Overhead() {
		return "", ErrCiphertextInvalid
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	aad := associatedData(channelID, channelType)
	if v2 {
		aad = associatedDataV2(channelID, channelType, keyID)
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", ErrCiphertextInvalid
	}
	defer zeroBytes(plaintext)
	credential := strings.TrimSpace(string(plaintext))
	if credential == "" {
		return "", ErrEmptyCredential
	}
	return credential, nil
}

// ActiveKeyID reports the key id new envelopes are sealed under.
func (r *Keyring) ActiveKeyID() string {
	if r == nil {
		return ""
	}
	return r.activeKeyID
}

// SealedWithActiveKey reports whether stored ciphertext already uses the
// active key, letting migrations skip rows that need no rewrite.
func (r *Keyring) SealedWithActiveKey(storedCredential string) bool {
	if r == nil || r.active == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(storedCredential), CiphertextPrefix+r.activeKeyID+":")
}

// Encrypt seals a credential under the active key.
func (r *Keyring) Encrypt(channelID, channelType, credential string) (string, error) {
	if r == nil || r.active == nil {
		return "", ErrKeyRequired
	}
	return r.active.Encrypt(channelID, channelType, credential)
}

// Decrypt opens an envelope using whichever ring key produced it.
func (r *Keyring) Decrypt(channelID, channelType, storedCredential string) (string, error) {
	if r == nil || r.active == nil {
		return "", ErrKeyRequired
	}
	storedCredential = strings.TrimSpace(storedCredential)
	if strings.HasPrefix(storedCredential, CiphertextPrefix) {
		keyID, _, ok := parseV2Envelope(storedCredential)
		if !ok {
			return "", ErrCiphertextInvalid
		}
		keyCipher := r.byID[keyID]
		if keyCipher == nil {
			return "", ErrCiphertextInvalid
		}
		return keyCipher.Decrypt(channelID, channelType, storedCredential)
	}
	if strings.HasPrefix(storedCredential, LegacyCiphertextPrefix) {
		for _, keyCipher := range r.legacyOrder {
			credential, err := keyCipher.Decrypt(channelID, channelType, storedCredential)
			if err == nil {
				return credential, nil
			}
		}
		return "", ErrCiphertextInvalid
	}
	if storedCredential == "" {
		return "", ErrEmptyCredential
	}
	return "", ErrPlaintext
}

func parseV2Envelope(value string) (string, string, bool) {
	remainder := strings.TrimPrefix(strings.TrimSpace(value), CiphertextPrefix)
	keyID, encoded, ok := strings.Cut(remainder, ":")
	if !ok || !ValidKeyID(keyID) || encoded == "" {
		return "", "", false
	}
	return keyID, encoded, true
}

// DerivedKeyID names a key by a stable digest prefix of the key material.
func DerivedKeyID(key []byte) string {
	sum := sha256.Sum256(key)
	return "key-" + hex.EncodeToString(sum[:8])
}

// ValidKeyID reports whether a key id is safe to embed in an envelope.
func ValidKeyID(keyID string) bool {
	if keyID == "" || len(keyID) > maximumKeyIDLength {
		return false
	}
	for _, character := range keyID {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func associatedData(channelID, channelType string) []byte {
	channelID = strings.TrimSpace(channelID)
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if channelType == "" {
		channelType = defaultChannelType
	}
	return []byte(aadDomain + channelType + "\x00" + channelID)
}

func associatedDataV2(channelID, channelType, keyID string) []byte {
	return append(associatedData(channelID, channelType), []byte("\x00key-id\x00"+keyID)...)
}

// IsEncrypted reports whether a stored value carries a known envelope prefix.
func IsEncrypted(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, CiphertextPrefix) ||
		strings.HasPrefix(value, LegacyCiphertextPrefix)
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
