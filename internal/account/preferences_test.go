package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPreferenceValidation(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		valid      bool
	}{
		{"general.enable_emoji", "true", true},
		{"platform.ios.nine_key", "true", true},
		{"platform.ios.sound_enabled", "false", true},
		{"platform.ios.haptics_enabled", `"true"`, false},
		{"platform.ios.dictionary_learning", "true", true},
		{"platform.ios.haptic_strength", `"medium"`, true},
		{"platform.ios.keyboard_skin", `"forest"`, true},
		{"platform.ios.custom_keyboard_skin", `"{\"background\":15266027}"`, true},
		{"platform.ios.custom_keyboard_skin", `{}`, false},
		{"platform.ios.access_token", `"credential"`, false},
		{"general.enable_emoji", " null ", false},
		{"general.enable_emoji", `"true"`, false},
		{"appearance.page_size", "5", true},
		{"appearance.page_size", "1.5", false},
		{"appearance.page_size", "-1", false},
		{"ai_assistant.api_key", `"credential"`, false},
		{"voice_input.asr_endpoint", `"https://example.test"`, false},
		{"utility.clipboard_history", "true", false},
		{"appearance.font", `"a\u0000b"`, false},
	} {
		if got := validPreference(tc.key, json.RawMessage(tc.value)); got != tc.valid {
			t.Errorf("%s %s: got %v", tc.key, tc.value, got)
		}
	}
}

func TestPreferencesRevisionIsolationAndDeletion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "prefs-one@example.test"})
	two := complete(t, s, Identity{"email", "prefs-two@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s})
	call := func(method, path, token, body string, expected int) Preferences {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var out Preferences
		if expected == 200 && !strings.HasSuffix(path, "/schema") {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	path := "/v1/users/me/preferences"
	call("GET", path, "device-token", "", 401)
	call("GET", path+"/schema", one.AccessToken, "", 200)
	initial := call("GET", path, one.AccessToken, "", 200)
	if initial.Revision != 0 || len(initial.Settings) != 0 {
		t.Fatal("new user preferences", initial)
	}
	for _, body := range []string{`{}`, `{"revision":0,"settings":null}`, `{"revision":0,"settings":{"ai_assistant.api_key":"secret"}}`, `{"revision":0,"settings":{"appearance.page_size":true}}`} {
		call("PUT", path, one.AccessToken, body, 400)
	}
	first := call("PUT", path, one.AccessToken, `{"revision":0,"settings":{"appearance.page_size":5,"general.enable_emoji":true,"platform.ios.nine_key":true,"platform.ios.haptic_strength":"medium"}}`, 200)
	if first.Revision != 1 || string(first.Settings["platform.ios.nine_key"]) != "true" || string(first.Settings["appearance.page_size"]) != "5" {
		t.Fatal(first)
	}
	call("PUT", path, one.AccessToken, `{"revision":0,"settings":{}}`, 409)
	isolated := call("GET", path, two.AccessToken, "", 200)
	if isolated.Revision != 0 || len(isolated.Settings) != 0 {
		t.Fatal("cross-user leak", isolated)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.PutPreferences(ctx, one.User.ID, 1, map[string]json.RawMessage{"appearance.page_size": json.RawMessage(`6`)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, errRevisionConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent CAS: %d success, %d conflict", success, conflict)
	}
	updated := call("GET", path, one.AccessToken, "", 200)
	if updated.Revision != 2 || len(updated.Settings) != 1 {
		t.Fatal("replace semantics", updated)
	}
	cleared := call("PUT", path, one.AccessToken, `{"revision":2,"settings":{}}`, 200)
	if cleared.Revision != 3 || len(cleared.Settings) != 0 {
		t.Fatal(cleared)
	}
	if err := s.DeleteUser(ctx, one.User.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM user_preferences WHERE user_id=$1", one.User.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("delete cascade", count, err)
	}
}
