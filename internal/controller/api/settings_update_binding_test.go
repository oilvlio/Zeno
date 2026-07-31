package api

import (
	"reflect"
	"testing"
)

// A sparse PATCH must upsert only the keys it carries. If it wrote every setting
// instead, two disjoint concurrent requests would overwrite each other with
// stale values.
func TestAdminSettingsUpdateValuesIsSparse(t *testing.T) {
	if values := adminSettingsUpdateValues(AdminSettingsUpdateRequest{}); len(values) != 0 {
		t.Fatalf("an empty PATCH must write nothing, got %v", values)
	}

	title := "Zeno"
	opacity := 0.5
	values := adminSettingsUpdateValues(AdminSettingsUpdateRequest{SiteTitle: &title, CardOpacity: &opacity})
	want := map[string]string{
		settingKeySiteTitle:   "Zeno",
		settingKeyCardOpacity: formatSettingsFloat(opacity),
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

// Empty strings and zero numbers are meaningful values, not absent fields:
// clearing a subtitle or setting opacity to 0 must persist.
func TestAdminSettingsUpdateValuesKeepsZeroValues(t *testing.T) {
	empty := ""
	zero := 0.0
	values := adminSettingsUpdateValues(AdminSettingsUpdateRequest{SiteSubtitle: &empty, CardBlur: &zero})
	if got, ok := values[settingKeySiteSubtitle]; !ok || got != "" {
		t.Fatalf("clearing the subtitle must persist an empty string, got %q ok=%v", got, ok)
	}
	if got, ok := values[settingKeyCardBlur]; !ok || got != formatSettingsFloat(0) {
		t.Fatalf("zero blur must persist, got %q ok=%v", got, ok)
	}
}

// background_url predates the desktop/mobile split, so the legacy key and the
// desktop key must never disagree; an explicit desktop value wins.
func TestAdminSettingsUpdateValuesBackgroundAliases(t *testing.T) {
	legacy := "https://example.com/legacy.png"
	desktop := "https://example.com/desktop.png"
	mobile := "https://example.com/mobile.png"

	onlyLegacy := adminSettingsUpdateValues(AdminSettingsUpdateRequest{BackgroundURL: &legacy})
	if onlyLegacy[settingKeyBackgroundURL] != legacy || onlyLegacy[settingKeyDesktopBackgroundURL] != legacy {
		t.Fatalf("legacy background must fill the desktop key: %v", onlyLegacy)
	}

	onlyDesktop := adminSettingsUpdateValues(AdminSettingsUpdateRequest{DesktopBackgroundURL: &desktop})
	if onlyDesktop[settingKeyBackgroundURL] != desktop || onlyDesktop[settingKeyDesktopBackgroundURL] != desktop {
		t.Fatalf("desktop background must fill the legacy key: %v", onlyDesktop)
	}

	both := adminSettingsUpdateValues(AdminSettingsUpdateRequest{BackgroundURL: &legacy, DesktopBackgroundURL: &desktop})
	if both[settingKeyBackgroundURL] != desktop || both[settingKeyDesktopBackgroundURL] != desktop {
		t.Fatalf("an explicit desktop value must win over the legacy field: %v", both)
	}

	// The mobile background is independent and must not touch the desktop keys.
	onlyMobile := adminSettingsUpdateValues(AdminSettingsUpdateRequest{MobileBackgroundURL: &mobile})
	if _, ok := onlyMobile[settingKeyBackgroundURL]; ok {
		t.Fatalf("mobile background must not write desktop keys: %v", onlyMobile)
	}
	if onlyMobile[settingKeyMobileBackgroundURL] != mobile {
		t.Fatalf("mobile background not persisted: %v", onlyMobile)
	}
}

// Every pointer field on the request must be represented in the binding tables.
// Without this, adding a setting to the API and forgetting to bind it would be
// silently accepted and never persisted -- the exact failure the table replaced
// 19 hand-written nil checks to avoid.
func TestAdminSettingsUpdateBindingsCoverEveryRequestField(t *testing.T) {
	// Fields handled by the alias helper rather than a plain binding.
	aliasHandled := map[string]struct{}{
		"BackgroundURL":        {},
		"DesktopBackgroundURL": {},
	}

	bound := map[string]struct{}{}
	requestType := reflect.TypeOf(AdminSettingsUpdateRequest{})

	// Build the set of bound fields by setting each field in turn and observing
	// which keys appear, which ties the check to real behaviour rather than to a
	// duplicated list of names.
	for index := 0; index < requestType.NumField(); index++ {
		field := requestType.Field(index)
		if field.Type.Kind() != reflect.Ptr {
			continue
		}
		if _, ok := aliasHandled[field.Name]; ok {
			bound[field.Name] = struct{}{}
			continue
		}
		request := reflect.New(requestType).Elem()
		value := reflect.New(field.Type.Elem())
		switch field.Type.Elem().Kind() {
		case reflect.String:
			value.Elem().SetString("probe-value")
		case reflect.Float64:
			value.Elem().SetFloat(0.25)
		default:
			t.Fatalf("field %s has unsupported kind %s; extend this test", field.Name, field.Type.Elem().Kind())
		}
		request.Field(index).Set(value)

		values := adminSettingsUpdateValues(request.Interface().(AdminSettingsUpdateRequest))
		if len(values) > 0 {
			bound[field.Name] = struct{}{}
		}
	}

	missing := []string{}
	for index := 0; index < requestType.NumField(); index++ {
		field := requestType.Field(index)
		if field.Type.Kind() != reflect.Ptr {
			continue
		}
		if _, ok := bound[field.Name]; !ok {
			missing = append(missing, field.Name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("request fields not persisted by any binding: %v", missing)
	}
}
