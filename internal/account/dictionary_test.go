package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestDictionaryTransactionsIsolationAndChangeFeed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "dictionary-one@example.test"})
	two := complete(t, s, Identity{"email", "dictionary-two@example.test"})
	entry := DictionaryEntry{Code: "ni'hao", Word: "你好", Weight: 10}
	first, err := s.EditDictionary(ctx, one.User.ID, "pinyin", "", 0, &entry)
	if err != nil || first.Revision != 1 || first.Replacement == nil {
		t.Fatal(first, err)
	}
	if _, err = s.EditDictionary(ctx, two.User.ID, "pinyin", first.Replacement.ID, 1, nil); !errors.Is(err, errDataNotFound) {
		t.Fatal("cross-user delete", err)
	}
	if _, err = s.EditDictionary(ctx, one.User.ID, "pinyin", "", 0, &entry); !errors.Is(err, errDictionaryDuplicate) {
		t.Fatal("duplicate", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replacement := entry
			replacement.Weight = 20
			_, err := s.EditDictionary(ctx, one.User.ID, "pinyin", first.Replacement.ID, 1, &replacement)
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
		t.Fatal("concurrent revision", success, conflict)
	}
	batch := []DictionaryEntry{{Code: "zhong'guo", Word: "中国", Weight: 10}, entry}
	if _, err = s.ImportDictionary(ctx, one.User.ID, "pinyin", batch); !errors.Is(err, errDictionaryDuplicate) {
		t.Fatal("import duplicate", err)
	}
	rows, _, err := s.DictionaryEntries(ctx, one.User.ID, "pinyin", "", 0, 200)
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 {
		t.Fatal("import atomicity", rows, err)
	}
	deleted, err := s.EditDictionary(ctx, one.User.ID, "pinyin", first.Replacement.ID, 2, nil)
	if err != nil || deleted.Revision != 3 || deleted.Previous == nil || deleted.Replacement != nil {
		t.Fatal(deleted, err)
	}
	changes, more, err := s.DictionaryChanges(ctx, one.User.ID, 0, 2)
	if err != nil || !more || len(changes) != 2 {
		t.Fatal(changes, more, err)
	}
	tail, more, err := s.DictionaryChanges(ctx, one.User.ID, 2, 2)
	if err != nil || more || len(tail) != 1 || tail[0].Replacement != nil {
		t.Fatal("tombstone", tail, err)
	}
	isolated, _, err := s.DictionaryChanges(ctx, two.User.ID, 0, 200)
	if err != nil || len(isolated) != 0 {
		t.Fatal("change feed leak", isolated, err)
	}
	if err = s.DeleteUser(ctx, one.User.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"user_dictionary_entries", "user_dictionary_changes", "user_dictionary_state"} {
		var count int
		if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE user_id=$1", one.User.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("delete cascade", table, count, err)
		}
	}
}
func TestDictionaryHTTPWithNativeValidation(t *testing.T) {
	binary := os.Getenv("MSIME_ENGINE_TEST_BINARY")
	if binary == "" {
		t.Skip("需要真实原生 Engine")
	}
	s := testStore(t)
	one := complete(t, s, Identity{"email", "native-dictionary@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s, engine: engine.Config{Binary: binary, Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}})
	call := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+one.AccessToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	base := "/v1/users/me/dictionaries/"
	for _, tc := range []struct{ kind, code, word string }{{"pinyin", "ni hao", "你好"}, {"wubi", "wq", "你"}, {"quick", "test", "测试短语"}, {"english", "hello", "Hello"}} {
		body, _ := json.Marshal(map[string]any{"code": tc.code, "word": tc.word, "weight": 10})
		response := call("POST", base+tc.kind, string(body), 201)
		var change DictionaryChange
		if err := json.Unmarshal(response.Body.Bytes(), &change); err != nil {
			t.Fatal(err)
		}
		if change.Replacement == nil {
			t.Fatal(change)
		}
		call("POST", base+tc.kind, string(body), 409)
		exported := call("GET", base+tc.kind+"/export", "", 200)
		if !strings.Contains(exported.Body.String(), "\t10\n") {
			t.Fatal(exported.Body.String())
		}
		call("PUT", base+tc.kind+"/"+change.Replacement.ID, `{"code":"bad","word":"x"}`, 400)
	}
	call("POST", base+"pinyin", `{"code":"garbage-code","word":"测试"}`, 400)
	call("POST", base+"pinyin/import", `{"text":"中国\tzhong guo\t20\n学习\txue xi\t30"}`, 200)
	call("POST", base+"pinyin/import", `{"text":"坏行"}`, 400)
	call("POST", base+"pinyin/import", `{"text":"学校\txue xiao\t20\n中国\tzhong guo\t30"}`, 409)
	empty := call("GET", base+"pinyin?q=xiao", "", 200)
	if strings.Contains(empty.Body.String(), "学校") {
		t.Fatal("partial import committed")
	}
	page := call("GET", base+"pinyin?limit=1", "", 200)
	if !strings.Contains(page.Body.String(), `"has_more":true`) {
		t.Fatal("missing pagination")
	}
	feed := call("GET", "/v1/users/me/dictionary/changes?limit=2", "", 200)
	if !strings.Contains(feed.Body.String(), `"has_more":true`) {
		t.Fatal(feed.Body.String())
	}
	call("GET", base+"pinyin?limit=201", "", 400)
	call("GET", base+"invalid", "", 404)
	if os.Getenv("MSIME_ENGINE_TEST_RESOURCES") != "" {
		call("POST", base+"pinyin/import-hans", `{"text":"重庆\n银行","weight":50}`, 200)
		exported := call("GET", base+"pinyin/export", "", 200)
		if !strings.Contains(exported.Body.String(), "重庆\tchong'qing\t50") || !strings.Contains(exported.Body.String(), "银行\tyin'hang\t50") {
			t.Fatal("phrase-aware annotation", exported.Body.String())
		}
		call("POST", base+"pinyin/import-hans", `{"text":"hello"}`, 400)
		call("POST", base+"quick/import-hans", `{"text":"你好"}`, 400)
	}

}
