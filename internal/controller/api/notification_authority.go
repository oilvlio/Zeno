package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shui1iao/zeno/internal/controller/notifycrypto"
)

const (
	settingKeyNotificationAuthorityFingerprint = "internal.notification_authority_fingerprint"
	settingKeyNotificationAuthorityKeyID       = "internal.notification_authority_key_id"
)

// AuthorizeNotificationAuthority preserves the original single-key API while
// using a deterministic key id in the persisted authority binding.
func (s *SQLiteStore) AuthorizeNotificationAuthority(ctx context.Context, authorityKey string) (bool, error) {
	authorityKey = strings.TrimSpace(authorityKey)
	if authorityKey == "" {
		return false, nil
	}
	keyID := notifycrypto.AuthorityDerivedKeyID(authorityKey)
	return s.AuthorizeNotificationAuthorityKeyring(ctx, keyID, map[string]string{keyID: authorityKey})
}

// AuthorizeNotificationAuthorityKeyring binds delivery authority to an
// external key ring without storing any key in SQLite. If the stored binding
// matches any supplied old key, it is atomically advanced to activeKeyID; a
// subsequent restart can safely remove the old key from the ring.
func (s *SQLiteStore) AuthorizeNotificationAuthorityKeyring(ctx context.Context, activeKeyID string, keys map[string]string) (bool, error) {
	keyring, configured, err := notifycrypto.NewAuthorityKeyring(activeKeyID, keys)
	if err != nil {
		return false, translateNotificationAuthorityError(err)
	}
	if !configured {
		return false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	nowUnix := time.Now().UTC().Unix()

	claimed, err := claimNotificationAuthority(ctx, tx, keyring.ActiveFingerprint(), nowUnix)
	if err != nil {
		return false, err
	}
	if claimed {
		if err := writeNotificationAuthorityKeyID(ctx, tx, keyring.ActiveKeyID(), nowUnix); err != nil {
			return false, err
		}
		return true, commitNotificationAuthority(&tx)
	}

	var storedFingerprint string
	if err := tx.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`,
		settingKeyNotificationAuthorityFingerprint,
	).Scan(&storedFingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("notification authority binding disappeared")
		}
		return false, err
	}

	if !keyring.Matches(storedFingerprint) {
		// Not an error: an unrelated controller simply does not own this
		// database. Commit so the read transaction does not linger.
		return false, commitNotificationAuthority(&tx)
	}

	// Matching an old authority proves continuity. Advance both fingerprint and
	// key id in one transaction; neither secret nor key material is persisted.
	if err := writeNotificationAuthoritySetting(ctx, tx,
		settingKeyNotificationAuthorityFingerprint, keyring.ActiveFingerprint(), nowUnix); err != nil {
		return false, err
	}
	if err := writeNotificationAuthorityKeyID(ctx, tx, keyring.ActiveKeyID(), nowUnix); err != nil {
		return false, err
	}
	return true, commitNotificationAuthority(&tx)
}

// claimNotificationAuthority attempts to install the binding on an unbound
// database. The insert runs before any read so concurrent controller startups
// serialize on the write: a read-then-upsert transaction could let two
// different keys both observe no binding and each report authorization.
func claimNotificationAuthority(ctx context.Context, tx *sql.Tx, fingerprint string, nowUnix int64) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO NOTHING
	`, settingKeyNotificationAuthorityFingerprint, fingerprint, nowUnix)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return inserted == 1, nil
}

func writeNotificationAuthorityKeyID(ctx context.Context, tx *sql.Tx, keyID string, nowUnix int64) error {
	return writeNotificationAuthoritySetting(ctx, tx, settingKeyNotificationAuthorityKeyID, keyID, nowUnix)
}

func writeNotificationAuthoritySetting(ctx context.Context, tx *sql.Tx, key, value string, nowUnix int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, nowUnix)
	return err
}

// commitNotificationAuthority commits and clears the caller's transaction
// handle so the deferred rollback becomes a no-op.
func commitNotificationAuthority(tx **sql.Tx) error {
	if err := (*tx).Commit(); err != nil {
		return err
	}
	*tx = nil
	return nil
}

// translateNotificationAuthorityError keeps the store's historical error text
// stable now that validation lives in notifycrypto.
func translateNotificationAuthorityError(err error) error {
	switch {
	case errors.Is(err, notifycrypto.ErrAuthorityKeyring):
		return fmt.Errorf("invalid notification authority key ring")
	case errors.Is(err, notifycrypto.ErrAuthorityActiveKeyMissing):
		return fmt.Errorf("active notification authority key id is not in key ring")
	default:
		return err
	}
}
