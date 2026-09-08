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

func TestSnapshotRestoreHTTP(t *testing.T) {
	cfg := engine.Config{Binary: os.Getenv("MSIME_ENGINE_TEST_BINARY"), Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}
	if cfg.Binary == "" || cfg.Resources == "" {
		t.Skip("需要真实 Engine 与发布词库")
	}
	s := testStore(t)
	user := complete(t, s, Identity{"email", "restore-http@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s, engine: cfg})
	snapshot := signedSnapshot(`{"type":"header","format":"msime-dictionary-snapshot","version":1,"revision":0}`)
	call := func(token, path, media string, body []byte, length int64, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("PUT", path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", media)
		if length >= 0 {
			r.ContentLength = length
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	path := "/v1/users/me/dictionary/snapshot?revision=0"
	media := "application/x-ndjson"
	call("device-token", path, media, snapshot, -1, 401)
	call(user.AccessToken, path, "application/json", snapshot, -1, 415)
	call(user.AccessToken, "/v1/users/me/dictionary/snapshot", media, snapshot, -1, 400)
	call(user.AccessToken, path+"&revision=1", media, snapshot, -1, 400)
	call(user.AccessToken, path, media, snapshot, snapshotRestoreBytes+1, 413)
	w := call(user.AccessToken, path, media, snapshot, -1, 200)
	var result struct {
		Revision int64 `json:"revision"`
		Reset    bool  `json:"reset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || !result.Reset || result.Revision <= 0 {
		t.Fatal(w.Body.String(), err)
	}
	call(user.AccessToken, path, media, snapshot, -1, 409)
	call(user.AccessToken, path, media, snapshot[:len(snapshot)-10], -1, 400)
	snapshotRestoreSlot <- struct{}{}
	busy := call(user.AccessToken, path, media, snapshot, -1, 503)
	<-snapshotRestoreSlot
	if busy.Header().Get("Retry-After") != "5" {
		t.Fatal("missing busy retry delay")
	}
	call(user.AccessToken, path, media, snapshot, -1, 409)
	call(user.AccessToken, path, media, snapshot, -1, 429)
	var revision int64
	if err := s.pool.QueryRow(context.Background(), `SELECT revision FROM user_dictionary_state WHERE user_id=$1`, user.User.ID).Scan(&revision); err != nil || revision != result.Revision {
		t.Fatal("rejected request changed state", revision, err)
	}
}
