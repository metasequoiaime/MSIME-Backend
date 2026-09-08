package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentationAndBrowserAuthentication(t *testing.T) {
	s := fixture(t, nil)
	for _, path := range []string{"/swagger/", "/swagger/swagger-ui-bundle.js", "/swagger/swagger-ui.css", "/swagger/init.js", "/openapi.json"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("documentation %s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	var spec struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if json.Unmarshal(w.Body.Bytes(), &spec) != nil || spec.OpenAPI != "3.0.3" || len(spec.Paths) != 7 {
		t.Fatal("incomplete OpenAPI")
	}
	for _, tc := range []struct {
		origin, token string
		status        int
	}{
		{"http://example.com", testToken, 200},
		{"https://example.com", testToken, 200},
		{"http://example.com", "wrong", 401},
		{"https://untrusted.example", testToken, 403},
	} {
		r := httptest.NewRequest("GET", "http://example.com/v1/capabilities", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("browser origin/auth status %d want %d", w.Code, tc.status)
		}
	}
	for _, path := range []string{"/swagger-private", "/v1/chat/completions"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader("{}")))
		if w.Code != 401 {
			t.Fatal("documentation bypassed API authentication")
		}
	}
}
