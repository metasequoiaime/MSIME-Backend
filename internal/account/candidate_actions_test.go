package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestCandidateActionsHTTPWithNativeEngine(t *testing.T) {
	cfg := engine.Config{Binary: os.Getenv("MSIME_ENGINE_TEST_BINARY"), Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}
	if cfg.Binary == "" || cfg.Resources == "" {
		t.Skip("需要真实 Engine 与发布词库")
	}
	s := testStore(t)
	ctx := context.Background()
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s, engine: cfg})
	call := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	type candidate struct {
		Code      string `json:"code"`
		Canonical string `json:"canonical_pinyin"`
		Word      string `json:"word"`
		Weight    int64  `json:"weight"`
	}
	type result struct {
		Context    string      `json:"context"`
		Revision   int64       `json:"revision"`
		Candidates []candidate `json:"candidates"`
	}
	query := func(token string, q PersonalQuery) result {
		t.Helper()
		var out result
		if err := json.Unmarshal(call("POST", "/v1/users/me/dictionary/candidates", token, q, 200), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	q := PersonalQuery{Text: "nihao", Limit: 20}
	// Every mode is tested through authenticated HTTP against the shipped dictionaries.
	for _, mode := range []string{"disabled", "pin", "halve", "linear", "promote", "force_top"} {
		t.Run(mode, func(t *testing.T) {
			u := complete(t, s, Identity{"email", fmt.Sprintf("actions-%s@example.test", mode)})
			modeQuery := PersonalQuery{Text: "shi", Limit: 20}
			before := query(u.AccessToken, modeQuery)
			if len(before.Candidates) < 7 {
				t.Fatal(before)
			}
			selected := before.Candidates[6]
			a := RankingAction{Code: selected.Canonical, Word: selected.Word, Mode: mode, LinearStep: 1, TriggerCount: 1}
			if mode == "force_top" {
				a.Mode = "disabled"
				a.ForceTop = true
			}
			body := map[string]any{"revision": before.Revision, "query": modeQuery, "action": a}
			path := "/v1/users/me/dictionary/ranking"
			call("POST", path, "device-token", body, 401)
			var ranked RankingResult
			if err := json.Unmarshal(call("POST", path, u.AccessToken, body, 200), &ranked); err != nil {
				t.Fatal(err)
			}
			after := query(u.AccessToken, modeQuery)
			if mode == "disabled" {
				if ranked.Changed || after.Candidates[0] != before.Candidates[0] {
					t.Fatal("disabled ranking changed", ranked, after)
				}
			} else {
				target := 0
				switch mode {
				case "halve":
					target = 3
				case "linear":
					target = 5
				case "promote":
					target = 4
				}
				if !ranked.Changed || after.Candidates[target].Word != selected.Word {
					t.Fatal("promotion failed", ranked, after)
				}
			}
			call("POST", path, u.AccessToken, body, 409)
			for _, entry := range ranked.Updates {
				if entry.Weight < 1 || entry.Weight > 100000000 {
					t.Fatal("unbounded weight", entry)
				}
			}
		})
	}
	u := complete(t, s, Identity{"email", "delete-base@example.test"})
	other := complete(t, s, Identity{"email", "delete-other@example.test"})
	before := query(u.AccessToken, q)
	selected := before.Candidates[0]
	removal := map[string]any{"revision": before.Revision, "query": q, "code": selected.Canonical, "word": selected.Word}
	path := "/v1/users/me/dictionary/candidates"
	call("DELETE", path, "device-token", removal, 401)
	var deleted DictionaryChange
	if err := json.Unmarshal(call("DELETE", path, u.AccessToken, removal, 200), &deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.Previous == nil || deleted.Previous.Word != selected.Word || deleted.Replacement != nil {
		t.Fatal("missing tombstone", deleted)
	}
	for _, c := range query(u.AccessToken, q).Candidates {
		if c.Word == selected.Word && c.Canonical == selected.Canonical {
			t.Fatal("deleted candidate replayed", c)
		}
	}
	if query(other.AccessToken, q).Candidates[0] != selected {
		t.Fatal("deletion leaked")
	}
	call("DELETE", path, u.AccessToken, removal, 409)
	removal["revision"] = deleted.Revision
	call("DELETE", path, u.AccessToken, removal, 400)
	if _, err := s.pool.Exec(ctx, `TRUNCATE user_dictionary_overlay`); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, c := range query(u.AccessToken, q).Candidates {
		if c.Word == selected.Word && c.Canonical == selected.Canonical {
			t.Fatal("deleted candidate restored during rebuild")
		}
	}
	single := PersonalQuery{Text: "ni", Limit: 20}
	singles := query(u.AccessToken, single)
	found := false
	for _, c := range singles.Candidates {
		if len([]rune(c.Word)) == 1 {
			removal["query"] = single
			removal["code"] = c.Canonical
			removal["word"] = c.Word
			call("DELETE", path, u.AccessToken, removal, 400)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no single-character fixture")
	}
}
