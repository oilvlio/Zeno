package api

import (
	"bytes"
	"strings"
	"testing"
)

func extractQuotedInstallCredential(t *testing.T, command string) string {
	t.Helper()
	marker := "ZENO_ENROLLMENT_TOKEN='"
	start := strings.Index(command, marker)
	if start < 0 {
		t.Fatalf("install command does not contain quoted credential: %s", command)
	}
	start += len(marker)
	end := strings.Index(command[start:], "'")
	if end < 0 {
		t.Fatalf("install credential quote not closed: %s", command)
	}
	return command[start : start+end]
}

func assertNoSensitiveAdminProbeTargetLeak(t *testing.T, raw string) {
	t.Helper()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("agent-super-secret")) {
		t.Fatalf("admin probe target response leaked sensitive fields: %s", raw)
	}
}

func assertNoSensitiveNotificationLeak(t *testing.T, raw string) {
	t.Helper()
	lower := bytes.ToLower([]byte(raw))
	if bytes.Contains(lower, []byte("token")) || bytes.Contains(lower, []byte("secret")) || bytes.Contains([]byte(raw), []byte("telegram-bot-secret-value")) || bytes.Contains([]byte(raw), []byte("email-secret-value")) {
		t.Fatalf("notification response leaked sensitive fields: %s", raw)
	}
}

func assertNoNotificationCredentialField(t *testing.T, raw string) {
	t.Helper()
	if bytes.Contains([]byte(raw), []byte(`"credential":`)) {
		t.Fatalf("notification response exposed write-only credential field: %s", raw)
	}
}

func stringPtr(value string) *string {
	return &value
}
