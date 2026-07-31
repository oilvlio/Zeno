package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/shui1iao/zeno/internal/controller/notifycrypto"
)

// Credential encryption itself lives in internal/controller/notifycrypto. This
// file keeps only the storage-facing adapter: keyring installation, snapshot
// access under the store lock, and the at-rest migration transaction.

// notificationCredentialCiphertextPrefix is referenced by storage-level tests
// that assert credentials are never persisted in the clear.
const notificationCredentialCiphertextPrefix = notifycrypto.CiphertextPrefix

var (
	errNotificationCredentialKeyRequired       = notifycrypto.ErrKeyRequired
	errNotificationCredentialCiphertextInvalid = notifycrypto.ErrCiphertextInvalid
)

type notificationCredentialState struct {
	mu      sync.RWMutex
	keyring *notifycrypto.Keyring
}

func newNotificationCredentialCipher(key []byte) (*notifycrypto.Cipher, error) {
	return notifycrypto.NewCipher(key)
}

// translateNotificationCredentialError maps the crypto package's blank
// credential signal onto the admin write error the HTTP layer already reports.
func translateNotificationCredentialError(err error) error {
	if err == notifycrypto.ErrEmptyCredential {
		return errInvalidAdminNotificationChannelWrite
	}
	return err
}

func (s *SQLiteStore) ConfigureNotificationCredentialEncryption(ctx context.Context, key []byte) error {
	keyID := notifycrypto.DerivedKeyID(key)
	return s.ConfigureNotificationCredentialKeyring(ctx, keyID, map[string][]byte{keyID: key})
}

// ConfigureNotificationCredentialKeyring installs a key-id-addressable ring.
// Existing ciphertext is decrypted with the supplied ring and atomically
// re-encrypted under activeKeyID. Callers can therefore remove retired keys on
// a later restart without asking administrators to re-enter every credential.
func (s *SQLiteStore) ConfigureNotificationCredentialKeyring(ctx context.Context, activeKeyID string, keys map[string][]byte) error {
	ring, err := notifycrypto.NewKeyring(activeKeyID, keys)
	if err != nil {
		return translateNotificationCredentialError(err)
	}
	if err := s.migrateNotificationCredentialsToEncrypted(ctx, ring); err != nil {
		return err
	}
	s.notificationCredentials.mu.Lock()
	s.notificationCredentials.keyring = ring
	s.notificationCredentials.mu.Unlock()
	return nil
}

func (s *SQLiteStore) RequireNotificationCredentialKeyForExistingCredentials(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite store is closed")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_channels WHERE TRIM(COALESCE(credential, '')) <> ''`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errNotificationCredentialKeyRequired
	}
	return nil
}

func (s *SQLiteStore) notificationCredentialKeyringSnapshot() *notifycrypto.Keyring {
	if s == nil {
		return nil
	}
	s.notificationCredentials.mu.RLock()
	ring := s.notificationCredentials.keyring
	s.notificationCredentials.mu.RUnlock()
	return ring
}

func (s *SQLiteStore) encryptNotificationCredentialForStorage(channelID, channelType, credential string) (string, error) {
	ring := s.notificationCredentialKeyringSnapshot()
	if ring == nil {
		return "", errNotificationCredentialKeyRequired
	}
	sealed, err := ring.Encrypt(channelID, channelType, credential)
	if err != nil {
		return "", translateNotificationCredentialError(err)
	}
	return sealed, nil
}

func (s *SQLiteStore) decryptNotificationCredentialFromStorage(channelID, channelType, storedCredential string) (string, error) {
	ring := s.notificationCredentialKeyringSnapshot()
	if ring == nil {
		return "", errNotificationCredentialKeyRequired
	}
	credential, err := ring.Decrypt(channelID, channelType, storedCredential)
	if err != nil {
		return "", translateNotificationCredentialError(err)
	}
	return credential, nil
}

// notificationCredentialRewrite records one row that must move to the active key.
type notificationCredentialRewrite struct {
	channelID string
	previous  string
	next      string
}

func (s *SQLiteStore) migrateNotificationCredentialsToEncrypted(ctx context.Context, keyring *notifycrypto.Keyring) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite store is closed")
	}
	if keyring == nil || keyring.ActiveKeyID() == "" {
		return errNotificationCredentialKeyRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()

	rewrites, err := collectNotificationCredentialRewrites(ctx, tx, keyring)
	if err != nil {
		return err
	}
	for _, rewrite := range rewrites {
		result, err := tx.ExecContext(ctx, `
			UPDATE notification_channels
			SET credential = ?
			WHERE id = ? AND credential = ?
		`, rewrite.next, rewrite.channelID, rewrite.previous)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("notification credential migration conflict")
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func collectNotificationCredentialRewrites(ctx context.Context, tx *sql.Tx, keyring *notifycrypto.Keyring) ([]notificationCredentialRewrite, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, credential
		FROM notification_channels
		WHERE TRIM(COALESCE(credential, '')) <> ''
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	rewrites := make([]notificationCredentialRewrite, 0)
	for rows.Next() {
		var channelID string
		var storedCredential string
		if err := rows.Scan(&channelID, &storedCredential); err != nil {
			return nil, err
		}
		trimmedCredential := strings.TrimSpace(storedCredential)
		plaintextCredential := trimmedCredential
		if notifycrypto.IsEncrypted(trimmedCredential) {
			if keyring.SealedWithActiveKey(trimmedCredential) {
				continue
			}
			decryptedCredential, err := keyring.Decrypt(channelID, "telegram", trimmedCredential)
			if err != nil {
				return nil, translateNotificationCredentialError(err)
			}
			plaintextCredential = decryptedCredential
		}
		encryptedCredential, err := keyring.Encrypt(channelID, "telegram", plaintextCredential)
		if err != nil {
			return nil, translateNotificationCredentialError(err)
		}
		rewrites = append(rewrites, notificationCredentialRewrite{
			channelID: channelID,
			previous:  storedCredential,
			next:      encryptedCredential,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rewrites, nil
}
