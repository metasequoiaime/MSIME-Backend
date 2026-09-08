package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestServiceLifecycleAndUnavailableDatabase(t *testing.T) {
	db := testStore(t)
	t.Setenv("SERVICE_TEST_DB", os.Getenv("MSIME_TEST_DATABASE_URL"))
	t.Setenv("SERVICE_TEST_PEPPER", strings.Repeat("p", 32))
	cfg := Config{Enabled: true, DatabaseEnv: "SERVICE_TEST_DB", PepperEnv: "SERVICE_TEST_PEPPER"}
	a, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.ConfigureEngine(engine.Config{})
	a.Close()
	a.Close()
	var disabled *Service
	disabled.Close()
	disabled.ConfigureEngine(engine.Config{})
	if _, err := disabled.Authenticate(t.Context(), "x"); err != ErrInvalid {
		t.Fatal(err)
	}
	if disabled, err = New(t.Context(), Config{}); err != nil || disabled != nil {
		t.Fatal(disabled, err)
	}
	if _, err = New(t.Context(), Config{Enabled: true}); err == nil {
		t.Fatal("missing config accepted")
	}
	t.Setenv("SERVICE_TEST_DB", "not-a-postgres-connection-string")
	if _, err = New(t.Context(), cfg); err == nil {
		t.Fatal("invalid DSN accepted")
	}
	if _, err := db.pool.Exec(t.Context(), `DROP TABLE user_preferences CASCADE`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVICE_TEST_DB", os.Getenv("MSIME_TEST_DATABASE_URL"))
	if _, err = New(t.Context(), cfg); err == nil {
		t.Fatal("missing migration accepted")
	}
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	a = &Service{store: db}
	for _, path := range []string{"overview", "users", "downloads", "crashes", "skins", "dictionaries", "replies", "audit", "users/missing", "skins/missing", "dictionaries/missing", "replies/missing"} {
		w := httptest.NewRecorder()
		a.AdminHTTP(w, httptest.NewRequest("GET", "/api/"+path, nil))
		if w.Code != 503 || strings.Contains(w.Body.String(), "closed pool") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		a.AdminMembersHTTP(w, jsonRequest(method, "/api/admins", `{"email":"member@example.test","action":"add"}`, ""), nil)
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.adminAction(w, jsonRequest("POST", "/api/actions", `{"id":"user","action":"revoke_sessions"}`, ""))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	a.Telemetry(w, jsonRequest("POST", "/v1/telemetry/events", `{"id":"failure-event-0001","kind":"download","platform":"ios","version":"1"}`, ""))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	disabled.Telemetry(w, httptest.NewRequest("POST", "/v1/telemetry/events", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func jsonRequest(method, path, body, token string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func TestReadOnlyDatabaseRejectsMutationsWithoutSuccess(t *testing.T) {
	db := testStore(t)
	user := complete(t, db, Identity{"email", "readonly@example.test"})
	config := db.pool.Config()
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := &Service{store: &Store{pool: pool}}
	for _, tc := range []struct {
		name, method, path, body string
		handler                  func(http.ResponseWriter, *http.Request)
	}{
		{"profile", "PATCH", "/v1/users/me", `{"display_name":"not committed"}`, a.update},
		{"logout", "POST", "/v1/auth/logout", `{}`, a.logout},
		{"delete", "DELETE", "/v1/users/me", "", a.delete},
		{"clipboard setting", "PUT", "/v1/users/me/clipboard/settings", `{"enabled":true}`, a.clipboardSettings},
		{"preferences", "PUT", "/v1/users/me/preferences", `{"revision":0,"settings":{}}`, a.preferences},
		{"admin action", "POST", "/api/actions", `{"id":"` + user.User.ID + `","action":"revoke_sessions"}`, a.adminAction},
		{"telemetry", "POST", "/v1/telemetry/events", `{"id":"readonly-event-0001","kind":"download","platform":"ios","version":"1"}`, a.Telemetry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, jsonRequest(tc.method, tc.path, tc.body, user.AccessToken))
			if w.Code != 503 || strings.Contains(w.Body.String(), "read-only") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	if _, err := db.Authenticate(context.Background(), user.AccessToken); err != nil {
		t.Fatal("failed logout revoked session", err)
	}
	profile, _, err := db.Me(t.Context(), user.User.ID)
	if err != nil || profile.DisplayName == "not committed" {
		t.Fatal("failed update committed", err)
	}
}
