package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The query and the decoder are both derived from siteSettingsBindings, which
// is the property that prevents a key from being selected but never decoded
// (the failure mode of the previous hand-maintained switch).
func TestSiteSettingsQueryMatchesBindings(t *testing.T) {
	bindings := siteSettingsBindings()
	query, args := siteSettingsQuery(bindings)

	if len(args) != len(bindings) {
		t.Fatalf("argument count %d does not match binding count %d", len(args), len(bindings))
	}
	if placeholders := strings.Count(query, "?"); placeholders != len(bindings) {
		t.Fatalf("placeholder count %d does not match binding count %d", placeholders, len(bindings))
	}
	decoder := newSettingsDecoder(bindings)
	if len(decoder) != len(bindings) {
		t.Fatalf("decoder has %d keys but there are %d bindings (duplicate key?)", len(decoder), len(bindings))
	}
	for index, binding := range bindings {
		if args[index] != binding.key {
			t.Fatalf("argument %d is %v, want %s", index, args[index], binding.key)
		}
		if _, ok := decoder[binding.key]; !ok {
			t.Fatalf("selected key %s has no decoder", binding.key)
		}
	}
}

// Every settings key must be decodable. A key present in the query but absent
// from the decoder would silently read as its default forever.
func TestSiteSettingsBindingsHaveNoBlankOrDuplicateKeys(t *testing.T) {
	seen := map[string]struct{}{}
	for _, binding := range siteSettingsBindings() {
		if strings.TrimSpace(binding.key) == "" {
			t.Fatal("binding with blank key")
		}
		if binding.apply == nil {
			t.Fatalf("binding %s has no apply function", binding.key)
		}
		if _, duplicate := seen[binding.key]; duplicate {
			t.Fatalf("duplicate binding key %s", binding.key)
		}
		seen[binding.key] = struct{}{}
	}
}

// Unknown keys must be ignored rather than rejected, so a downgrade can read a
// database written by a newer build.
func TestSettingsDecoderIgnoresUnknownKeys(t *testing.T) {
	settings := defaultSiteSettings()
	before := settings
	newSettingsDecoder(siteSettingsBindings()).decode(&settings, "site.key_from_the_future", "value")
	if settings != before {
		t.Fatal("unknown key must not modify settings")
	}
}

// Invalid stored values fall back to the default instead of reaching the UI.
func TestSettingsDecoderRejectsInvalidValues(t *testing.T) {
	decoder := newSettingsDecoder(siteSettingsBindings())

	settings := defaultSiteSettings()
	decoder.decode(&settings, settingKeyThemeColor, "not-a-color")
	if settings.ThemeColor != defaultSiteSettings().ThemeColor {
		t.Fatalf("invalid theme color was stored: %s", settings.ThemeColor)
	}

	settings = defaultSiteSettings()
	decoder.decode(&settings, settingKeyAppearancePreset, "no-such-preset")
	if settings.AppearancePreset != defaultSiteSettings().AppearancePreset {
		t.Fatalf("invalid preset was stored: %s", settings.AppearancePreset)
	}

	settings = defaultSiteSettings()
	decoder.decode(&settings, settingKeyCardOpacity, "not-a-number")
	if settings.CardOpacity != defaultSiteSettings().CardOpacity {
		t.Fatalf("unparsable float overwrote the default: %v", settings.CardOpacity)
	}
}

// Valid values are applied to the field the binding names.
func TestSettingsDecoderAppliesValues(t *testing.T) {
	decoder := newSettingsDecoder(siteSettingsBindings())
	settings := defaultSiteSettings()

	decoder.decode(&settings, settingKeySiteTitle, "Zeno Ops")
	decoder.decode(&settings, settingKeyCustomCode, "<!-- hi -->")
	decoder.decode(&settings, settingKeyCardOpacity, "0.42")

	if settings.SiteTitle != "Zeno Ops" {
		t.Fatalf("title not applied: %s", settings.SiteTitle)
	}
	if settings.CustomCode != "<!-- hi -->" {
		t.Fatalf("custom code not applied: %s", settings.CustomCode)
	}
	if settings.CardOpacity != 0.42 {
		t.Fatalf("opacity not applied: %v", settings.CardOpacity)
	}
}

// End-to-end: a value written through the admin path must survive a reload,
// proving the binding table drives both directions consistently.
func TestSiteSettingsRoundTripThroughStore(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	title := "Round Trip"
	if _, err := store.UpdateAdminSettings(ctx, AdminSettingsUpdateRequest{SiteTitle: &title}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	loaded, err := store.siteSettings(ctx)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if loaded.SiteTitle != title {
		t.Fatalf("round trip lost the title: %s", loaded.SiteTitle)
	}
}
