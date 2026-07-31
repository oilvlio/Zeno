package api

import (
	"context"
	"database/sql"
	"time"
)

const (
	settingKeySiteTitle            = "site_title"
	settingKeySiteSubtitle         = "site_subtitle"
	settingKeyLogoURL              = "logo_url"
	settingKeyTheme                = "theme"
	settingKeyAgentControllerURL   = "agent_controller_url"
	settingKeyBackgroundURL        = "background_url"
	settingKeyDesktopBackgroundURL = "desktop_background_url"
	settingKeyMobileBackgroundURL  = "mobile_background_url"
	settingKeyAppearancePreset     = "appearance_preset"
	settingKeyCardOpacity          = "card_opacity"
	settingKeyCardBlur             = "card_blur"
	settingKeyCardRadius           = "card_radius"
	settingKeyBorderStrength       = "border_strength"
	settingKeyShadowStrength       = "shadow_strength"
	settingKeyBackgroundOverlay    = "background_overlay"
	settingKeyThemeColor           = "theme_color"
	settingKeyCustomCode           = "custom_code"
)

type sqliteSettings struct {
	db *sql.DB
}

func (s *sqliteSettings) PublicSettings(ctx context.Context) (SiteSettings, error) {
	return s.siteSettings(ctx)
}

func (s *sqliteSettings) AdminSettings(ctx context.Context) (SiteSettings, error) {
	return s.siteSettings(ctx)
}

func (s *sqliteSettings) UpdateAdminSettings(ctx context.Context, update AdminSettingsUpdateRequest) (SiteSettings, error) {
	if err := update.normalize(); err != nil {
		return SiteSettings{}, err
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SiteSettings{}, err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	values := adminSettingsUpdateValues(update)
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			return SiteSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SiteSettings{}, err
	}
	tx = nil
	// Re-read after commit so the response includes concurrent disjoint updates
	// rather than the stale pre-PATCH snapshot.
	return s.siteSettings(ctx)
}

func (s *sqliteSettings) siteSettings(ctx context.Context) (SiteSettings, error) {
	settings := defaultSiteSettings()
	bindings := siteSettingsBindings()
	query, args := siteSettingsQuery(bindings)
	decoder := newSettingsDecoder(bindings)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return SiteSettings{}, err
	}
	defer rows.Close()
	var latest sql.NullInt64
	for rows.Next() {
		var key, value string
		var updatedAt sql.NullInt64
		if err := rows.Scan(&key, &value, &updatedAt); err != nil {
			return SiteSettings{}, err
		}
		decoder.decode(&settings, key, value)
		if updatedAt.Valid && (!latest.Valid || updatedAt.Int64 > latest.Int64) {
			latest = updatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return SiteSettings{}, err
	}
	if settings.DesktopBackgroundURL == "" {
		settings.DesktopBackgroundURL = settings.BackgroundURL
	}
	if settings.BackgroundURL == "" {
		settings.BackgroundURL = settings.DesktopBackgroundURL
	}
	if latest.Valid && latest.Int64 > 0 {
		settings.UpdatedAt = time.Unix(latest.Int64, 0).UTC().Format(time.RFC3339)
	}
	return settings, nil
}
