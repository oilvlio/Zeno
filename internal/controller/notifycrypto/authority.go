package notifycrypto

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

// ErrAuthorityKeyring reports a malformed authority key ring. Callers map it
// to their own transport-level error; it is deliberately opaque so a bad key
// id or blank key cannot be distinguished from the outside.
var ErrAuthorityKeyring = errors.New("invalid notification authority key ring")

// ErrAuthorityActiveKeyMissing reports that the declared active key id is not
// present in the supplied ring, which would otherwise bind the database to a
// key the controller cannot actually use.
var ErrAuthorityActiveKeyMissing = errors.New("active notification authority key id is not in key ring")

// AuthorityKeyring is the validated, secret-free view of a notification
// authority key ring: only fingerprints survive, so nothing derived from it
// can leak key material into SQLite or logs.
//
// Fingerprints are ordered by key id to keep continuity checks deterministic
// regardless of map iteration order.
type AuthorityKeyring struct {
	activeKeyID       string
	activeFingerprint string
	keyIDs            []string
	fingerprints      map[string]string
}

// NewAuthorityKeyring validates the ring and precomputes fingerprints.
//
// It reports ok=false with a nil error for the "nothing configured" case so
// callers can treat an absent authority as unauthorized rather than an error.
func NewAuthorityKeyring(activeKeyID string, keys map[string]string) (*AuthorityKeyring, bool, error) {
	normalizedActiveKeyID := strings.TrimSpace(activeKeyID)
	if normalizedActiveKeyID == "" && len(keys) == 0 {
		return nil, false, nil
	}
	// Surrounding whitespace is rejected rather than trimmed: an id that only
	// matches after trimming would silently alias a different stored binding.
	if normalizedActiveKeyID != activeKeyID || !ValidKeyID(normalizedActiveKeyID) || len(keys) == 0 {
		return nil, false, ErrAuthorityKeyring
	}

	fingerprints := make(map[string]string, len(keys))
	keyIDs := make([]string, 0, len(keys))
	for rawKeyID, key := range keys {
		keyID := strings.TrimSpace(rawKeyID)
		key = strings.TrimSpace(key)
		if keyID != rawKeyID || !ValidKeyID(keyID) || key == "" {
			return nil, false, ErrAuthorityKeyring
		}
		if _, duplicate := fingerprints[keyID]; duplicate {
			return nil, false, ErrAuthorityKeyring
		}
		fingerprints[keyID] = AuthorityFingerprint(key)
		keyIDs = append(keyIDs, keyID)
	}
	sort.Strings(keyIDs)

	activeFingerprint, ok := fingerprints[normalizedActiveKeyID]
	if !ok {
		return nil, false, ErrAuthorityActiveKeyMissing
	}
	return &AuthorityKeyring{
		activeKeyID:       normalizedActiveKeyID,
		activeFingerprint: activeFingerprint,
		keyIDs:            keyIDs,
		fingerprints:      fingerprints,
	}, true, nil
}

// ActiveKeyID is the key id to persist alongside the binding.
func (r *AuthorityKeyring) ActiveKeyID() string {
	if r == nil {
		return ""
	}
	return r.activeKeyID
}

// ActiveFingerprint is the value stored as the authority binding.
func (r *AuthorityKeyring) ActiveFingerprint() string {
	if r == nil {
		return ""
	}
	return r.activeFingerprint
}

// Matches reports whether any key in the ring produced the stored
// fingerprint, which is what proves continuity with the existing binding.
//
// Every candidate is compared in constant time and the loop is not
// short-circuited, so comparison cost does not reveal which key matched.
func (r *AuthorityKeyring) Matches(storedFingerprint string) bool {
	if r == nil {
		return false
	}
	matched := false
	for _, keyID := range r.keyIDs {
		candidate := r.fingerprints[keyID]
		if len(candidate) == len(storedFingerprint) &&
			subtle.ConstantTimeCompare([]byte(candidate), []byte(storedFingerprint)) == 1 {
			matched = true
		}
	}
	return matched
}

// AuthorityFingerprint hashes an authority key. Only this digest is ever
// persisted, so the database never contains recoverable key material.
func AuthorityFingerprint(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

// AuthorityDerivedKeyID names the implicit key id used by the single-key API,
// so a legacy deployment and an explicit one-key ring bind identically.
func AuthorityDerivedKeyID(key string) string {
	return "authority-" + AuthorityFingerprint(key)[:16]
}
