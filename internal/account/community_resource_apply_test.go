package account

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestResourceApplyAtomicVersionedMerge(t *testing.T) {
	binary := os.Getenv("MSIME_ENGINE_TEST_BINARY")
	if binary == "" {
		t.Skip("native engine required")
	}
	s := testStore(t)
	owner := complete(t, s, Identity{"apple", "pack-owner"})
	reader := complete(t, s, Identity{"apple", "pack-reader"})
	a := &Service{store: s, engine: engine.Config{Binary: binary, Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}}
	mux := http.NewServeMux()
	Mount(mux, a)
	call := func(path, body, token string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
		return w.Body.Bytes()
	}
	id := "ab334455-1234-1234-1234-123456789abc"
	path := "/v1/community/resources/" + id + "/apply"
	call("/v1/community/resources", `{"id":"`+id+`","kind":"dictionary","name":"混合词包","description":"","content":{"entries":[{"kind":"pinyin","code":"ni hao","word":"你好","weight":20},{"kind":"quick","code":"hw","word":"Hello world","weight":10}]},"revision":0}`, owner.AccessToken, 201)
	body := `{"resource_revision":1,"dictionary_revision":0}`
	call(path, body, "", 401)
	call(path, `{}`, reader.AccessToken, 400)
	call(path, `{"resource_revision":2,"dictionary_revision":0}`, reader.AccessToken, 409)
	call(path, `{"resource_revision":1,"dictionary_revision":99}`, reader.AccessToken, 409)
	// Force a failure on the second kind, after the first edit has written its
	// entry, overlay and change. The transaction must roll back every write.
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `ALTER TABLE user_dictionary_entries ADD CONSTRAINT test_pack_failure CHECK (kind <> 'quick')`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `ALTER TABLE user_dictionary_entries DROP CONSTRAINT IF EXISTS test_pack_failure`)
	})
	call(path, body, reader.AccessToken, 503)
	var count int
	for _, table := range []string{"user_dictionary_entries", "user_dictionary_overlay", "user_dictionary_changes", "user_dictionary_state"} {
		if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE user_id=$1", reader.User.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial import in %s: %d %v", table, count, err)
		}
	}
	if _, err = s.pool.Exec(ctx, `ALTER TABLE user_dictionary_entries DROP CONSTRAINT test_pack_failure`); err != nil {
		t.Fatal(err)
	}
	result := call(path, body, reader.AccessToken, 200)
	var response struct {
		Revision int64
		Imported int
	}
	if json.Unmarshal(result, &response) != nil || response.Imported != 2 || response.Revision != 2 {
		t.Fatal(string(result))
	}
	call(path, body, reader.AccessToken, 409)
	result = call(path, `{"resource_revision":1,"dictionary_revision":2}`, reader.AccessToken, 200)
	if json.Unmarshal(result, &response) != nil || response.Imported != 0 || response.Revision != 2 {
		t.Fatal(string(result))
	}
	entries, _, err := s.DictionaryEntries(ctx, reader.User.ID, "pinyin", "", 0, 20)
	if err != nil || len(entries) != 1 || entries[0].Code != "ni'hao" {
		t.Fatal(entries, err)
	}
	changes, _, err := s.DictionaryChanges(ctx, reader.User.ID, 0, 20)
	if err != nil || len(changes) != 2 {
		t.Fatal(changes, err)
	}
	// Applying again updates an existing weight but preserves unrelated words.
	_, err = s.EditDictionary(ctx, reader.User.ID, "pinyin", entries[0].ID, entries[0].Revision, &DictionaryEntry{Code: entries[0].Code, Word: entries[0].Word, Weight: 7})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.EditDictionary(ctx, reader.User.ID, "quick", "", 0, &DictionaryEntry{Code: "keep", Word: "保留", Weight: 4})
	if err != nil {
		t.Fatal(err)
	}
	result = call(path, `{"resource_revision":1,"dictionary_revision":4}`, reader.AccessToken, 200)
	if json.Unmarshal(result, &response) != nil || response.Imported != 1 || response.Revision != 5 {
		t.Fatal(string(result))
	}
	entries, _, err = s.DictionaryEntries(ctx, reader.User.ID, "quick", "keep", 0, 20)
	if err != nil || len(entries) != 1 || entries[0].Word != "保留" {
		t.Fatal(entries, err)
	}
	entries, _, err = s.DictionaryEntries(ctx, owner.User.ID, "quick", "", 0, 20)
	if err != nil || len(entries) != 0 {
		t.Fatal("owner modified", entries, err)
	}
}
