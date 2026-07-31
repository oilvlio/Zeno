package api

import (
	"encoding/json"
	"testing"
)

// Admin PATCH validation is a security boundary: normalize() decides which
// partial updates reach SQLite. These cases pin the exact contract shared by
// the patchNormalizer helpers, including which field wins when several are
// invalid, so future refactors cannot quietly widen what is accepted.
func TestAdminNodeUpdateNormalizeContract(t *testing.T) {
	testCases := []struct {
		name      string
		payload   string
		wantError bool
		expect    func(t *testing.T, request AdminNodeUpdateRequest)
	}{
		{
			name:      "empty patch rejected",
			payload:   `{}`,
			wantError: true,
		},
		{
			name:    "display name trimmed",
			payload: `{"display_name":"  edge  "}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.DisplayName == nil || *request.DisplayName != "edge" {
					t.Fatalf("display name = %v, want trimmed", request.DisplayName)
				}
			},
		},
		{
			name:      "blank display name rejected",
			payload:   `{"display_name":"   "}`,
			wantError: true,
		},
		{
			name:    "country code upper cased",
			payload: `{"country_code":" jp "}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.CountryCode == nil || *request.CountryCode != "JP" {
					t.Fatalf("country code = %v, want JP", request.CountryCode)
				}
			},
		},
		{
			name:      "over-long country code rejected",
			payload:   `{"country_code":"toolongcountry"}`,
			wantError: true,
		},
		{
			name:    "blank region allowed as clear",
			payload: `{"region":"   "}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.Region == nil || *request.Region != "" {
					t.Fatalf("region = %v, want cleared", request.Region)
				}
			},
		},
		{
			name:      "explicit zero reset day rejected",
			payload:   `{"monthly_reset_day":0}`,
			wantError: true,
		},
		{
			name:      "out-of-range reset day rejected",
			payload:   `{"monthly_reset_day":32}`,
			wantError: true,
		},
		{
			name:      "negative display order rejected",
			payload:   `{"display_order":-1}`,
			wantError: true,
		},
		{
			name:    "zero display order accepted",
			payload: `{"display_order":0}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.DisplayOrder == nil || *request.DisplayOrder != 0 {
					t.Fatalf("display order = %v, want 0", request.DisplayOrder)
				}
			},
		},
		{
			name:      "mismatched ip family rejected",
			payload:   `{"public_ipv4":"2001:db8::1"}`,
			wantError: true,
		},
		{
			name:      "negative quota rejected",
			payload:   `{"monthly_quota_bytes":-1}`,
			wantError: true,
		},
		{
			name:    "explicit null quota clears value",
			payload: `{"monthly_quota_bytes":null}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if !request.MonthlyQuotaBytes.Set || request.MonthlyQuotaBytes.Valid {
					t.Fatalf("quota = %+v, want set-but-null", request.MonthlyQuotaBytes)
				}
			},
		},
		{
			name:    "boolean-only patch counts as a change",
			payload: `{"disabled":true}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.Disabled == nil || !*request.Disabled {
					t.Fatalf("disabled = %v, want true", request.Disabled)
				}
			},
		},
		{
			name:    "empty probe selection counts as a change",
			payload: `{"probe_target_ids":[]}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if request.ProbeTargetIDs == nil || len(request.ProbeTargetIDs) != 0 {
					t.Fatalf("probe targets = %v, want empty non-nil", request.ProbeTargetIDs)
				}
			},
		},
		{
			name:    "probe target ids trimmed",
			payload: `{"probe_target_ids":[" a ","b"]}`,
			expect: func(t *testing.T, request AdminNodeUpdateRequest) {
				if len(request.ProbeTargetIDs) != 2 || request.ProbeTargetIDs[0] != "a" || request.ProbeTargetIDs[1] != "b" {
					t.Fatalf("probe targets = %v, want trimmed [a b]", request.ProbeTargetIDs)
				}
			},
		},
		{
			name:      "duplicate probe target ids rejected",
			payload:   `{"probe_target_ids":["a","a"]}`,
			wantError: true,
		},
		{
			name:      "blank probe target id rejected",
			payload:   `{"probe_target_ids":["a","  "]}`,
			wantError: true,
		},
		{
			name:      "one invalid field rejects the whole patch",
			payload:   `{"display_name":" valid ","country_code":"toolongcountry"}`,
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var request AdminNodeUpdateRequest
			if err := json.Unmarshal([]byte(testCase.payload), &request); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			err := request.normalize()
			if testCase.wantError {
				if err != errInvalidAdminNodeUpdate {
					t.Fatalf("normalize error = %v, want errInvalidAdminNodeUpdate", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if testCase.expect != nil {
				testCase.expect(t, request)
			}
		})
	}
}

// A rejected patch must not leave partially normalized values behind, or a
// caller that ignores the error could persist half-validated input.
func TestAdminNodeUpdateNormalizeLeavesLaterFieldsUntouchedOnFailure(t *testing.T) {
	var request AdminNodeUpdateRequest
	payload := `{"country_code":"toolongcountry","region":"  east  "}`
	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if err := request.normalize(); err != errInvalidAdminNodeUpdate {
		t.Fatalf("normalize error = %v, want errInvalidAdminNodeUpdate", err)
	}
	if request.Region == nil || *request.Region != "  east  " {
		t.Fatalf("region = %v, want untouched after earlier failure", request.Region)
	}
}
