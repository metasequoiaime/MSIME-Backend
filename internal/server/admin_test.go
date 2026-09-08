package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminHostAuthAndIsolation(t *testing.T) {
	s := fixture(t, func(http.ResponseWriter, *http.Request) {})
	s.config.Admin = AdminConfig{Enabled: true, Host: "admin.msime.app", token: strings.Repeat("a", 40)}
	for _, tc := range []struct {
		host, path, token, origin string
		status                    int
	}{
		{"admin.msime.app", "/", "", "", 200},
		{"ADMIN.MSIME.APP:8080", "/users", "", "", 200},
		{"admin.msime.app", "/crashes", "", "", 200},
		{"admin.msime.app", "/api/overview", "", "", 401},
		{"admin.msime.app", "/api/skins/example", "", "", 401},
		{"admin.msime.app", "/api/dictionaries/example", testToken, "", 401},
		{"admin.msime.app", "/api/replies/example", strings.Repeat("a", 40), "https://evil.test", 403},
		{"api.msime.app", "/api/skins/example", testToken, "", 404},
		{"admin.msime.app", "/api/overview", testToken, "", 401},
		{"admin.msime.app", "/api/overview", strings.Repeat("a", 40), "https://evil.test", 403},
		{"api.msime.app", "/api/overview", strings.Repeat("a", 40), "", 401},
		{"api.msime.app", "/api/overview", testToken, "", 404},
		{"admin.msime.app", "/v1/capabilities", testToken, "", 404},
		{"admin.msime.app", "/web/index.html", "", "", 404},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
		if tc.status == 200 && (!strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal(w.Header())
		}
	}
	s.config.Admin.Enabled = false
	r := httptest.NewRequest("GET", "http://admin.msime.app/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("disabled admin is exposed")
	}
}

func TestAdminConfigValidation(t *testing.T) {
	t.Setenv("ADMIN_TEST", strings.Repeat("a", 40))
	t.Setenv("CLIENT_TEST", testToken)
	c := AdminConfig{Enabled: true, TokenEnv: "ADMIN_TEST"}
	if err := c.validate(true, []Client{{TokenEnv: "CLIENT_TEST"}}); err != nil || c.Host != "admin.msime.app" {
		t.Fatal(c, err)
	}
	if err := c.validate(false, nil); err == nil {
		t.Fatal("admin requires database")
	}
	c.Host = "https://admin.msime.app"
	if err := c.validate(true, nil); err == nil {
		t.Fatal("URL accepted as host")
	}
	c.Host = "admin.msime.app"
	c.TokenEnv = "CLIENT_TEST"
	if err := c.validate(true, []Client{{TokenEnv: "CLIENT_TEST"}}); err == nil {
		t.Fatal("shared token accepted")
	}
}
