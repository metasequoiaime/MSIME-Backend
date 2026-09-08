package account

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDictionaryHTTPFieldBoundariesAndDisabledEngine(t *testing.T) {
	db := testStore(t)
	user := complete(t, db, Identity{"email", "boundaries@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: db})
	base := "/v1/users/me/"
	for _, tc := range []struct {
		method, path, body, code string
		status                   int
	}{
		{"GET", "dictionaries/quick?offset=bad", "", "invalid_dictionary_query", 400},
		{"GET", "dictionaries/quick?limit=bad", "", "invalid_dictionary_query", 400},
		{"DELETE", "dictionaries/quick/id", `{"revision":1,"code":"x"}`, "invalid_delete", 400},
		{"POST", "dictionaries/missing/import", `{}`, "not_found", 404},
		{"GET", "dictionaries/missing/export", "", "not_found", 404},
		{"POST", "dictionaries/quick/import", `{"text":"hello\thi\tbad"}`, "invalid_dictionary_weight", 400},
		{"POST", "dictionaries/quick/import", `{"text":"hello\thi\t-1"}`, "invalid_dictionary_weight", 400},
		{"POST", "dictionaries/quick/import", `{"text":""}`, "dictionary_import_limit", 400},
		{"POST", "dictionaries/quick/import", `{"text":"hello\thi\t1"}`, "engine_unavailable", 503},
		{"GET", "dictionary/changes?after=-1", "", "invalid_change_cursor", 400},
		{"GET", "dictionary/changes?after=bad", "", "invalid_change_cursor", 400},
		{"POST", "dictionaries/pinyin/import-hans", `{"text":"\n \n"}`, "invalid_han_import", 400},
		{"POST", "dictionaries/pinyin/import-hans", `{"text":"你好","weight":-1}`, "invalid_han_import", 400},
		{"POST", "dictionaries/pinyin/import-hans", `{"text":"你好"}`, "engine_unavailable", 503},
		{"POST", "dictionaries/missing/edit", `{}`, "unknown_dictionary_kind", 404},
		{"POST", "dictionaries/quick/edit", `{}`, "invalid_dictionary_edit", 400},
		{"POST", "dictionaries/quick/edit", `{"revision":1,"previous":{"code":"hi","word":"hello"},"replacement":{"code":"hi","word":"hello"}}`, "invalid_dictionary_edit", 400},
		{"POST", "dictionaries/quick/edit", `{"revision":1,"previous":{"code":"hi","word":"hello"},"replacement":{"code":"hi","word":"","weight":1}}`, "invalid_dictionary_entry", 400},
		{"POST", "dictionary/ranking", `{}`, "invalid_ranking_action", 400},
		{"POST", "dictionary/ranking", `{"revision":0,"query":{"text":"ni"},"action":{"code":"ni","word":"你","mode":"bad"}}`, "invalid_ranking_mode", 400},
		{"POST", "dictionary/ranking", `{"revision":0,"query":{"text":"ni"},"action":{"code":"ni","word":"你","linear_step":101}}`, "invalid_ranking_action", 400},
		{"POST", "dictionary/ranking", `{"revision":0,"query":{"text":"!"},"action":{"code":"ni","word":"你"}}`, "invalid_input_code", 400},
		{"DELETE", "dictionary/candidates", `{}`, "invalid_candidate_removal", 400},
		{"DELETE", "dictionary/candidates", `{"revision":0,"query":{"text":"!"},"code":"ni","word":"你"}`, "invalid_input_code", 400},
		{"GET", "dictionary/positions?context=%00", "", "invalid_position_query", 400},
		{"DELETE", "dictionary/positions", `{"revision":0,"context":"hi","code":"hi","word":"hello","position":1}`, "invalid_candidate_position", 400},
	} {
		t.Run(tc.path+tc.code, func(t *testing.T) {
			w := apiRequest(t, mux, tc.method, base+tc.path, tc.body, user.AccessToken, tc.status)
			if !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatal(w.Body.String())
			}
		})
	}
	raw, _ := json.Marshal(map[string]string{"text": strings.Repeat("汉", 129)})
	w := apiRequest(t, mux, "POST", base+"dictionaries/pinyin/import-hans", string(raw), user.AccessToken, 400)
	if !strings.Contains(w.Body.String(), "han_phrase_too_long") {
		t.Fatal(w.Body.String())
	}
	// Rejected writes must not create a dictionary revision or changefeed entry.
	changes, _, err := db.DictionaryChanges(t.Context(), user.User.ID, 0, 20)
	if err != nil || len(changes) != 0 {
		t.Fatal(changes, err)
	}
}
