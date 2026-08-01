package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsTrailingContent(t *testing.T) {
	cases := []string{
		`{"name":"first"}{"name":"second"}`,
		`{"name":"first"} trailing`,
	}
	for _, body := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var payload struct {
			Name string `json:"name"`
		}
		if decodeJSONBody(recorder, request, &payload, 1024, true) {
			t.Fatalf("decodeJSONBody accepted trailing content %q", body)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for %q", recorder.Code, body)
		}
	}
}

func TestDecodeJSONBodyAllowsTrailingWhitespace(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{\"name\":\"ok\"}\n\t "))
	var payload struct {
		Name string `json:"name"`
	}
	if !decodeJSONBody(recorder, request, &payload, 1024, true) {
		t.Fatalf("decodeJSONBody rejected valid JSON with whitespace; status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if payload.Name != "ok" {
		t.Fatalf("decoded name = %q, want ok", payload.Name)
	}
}
