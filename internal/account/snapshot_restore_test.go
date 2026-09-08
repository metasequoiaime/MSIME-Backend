package account

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestRestoreFullDictionarySnapshotAtomically(t *testing.T) {
	cfg := engine.Config{Binary: os.Getenv("MSIME_ENGINE_TEST_BINARY"), Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}
	if cfg.Binary == "" || cfg.Resources == "" {
		t.Skip("需要真实 Engine 与发布词库")
	}
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "restore-source@example.test"})
	two := complete(t, s, Identity{"email", "restore-target@example.test"})
	created, err := s.EditDictionary(ctx, one.User.ID, "pinyin", "", 0, &DictionaryEntry{Code: "ni'hao", Word: "拟好", Weight: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetCandidatePosition(ctx, one.User.ID, 1, CandidatePosition{Context: "ni'hao", Code: "ni'hao", Word: "拟好", Position: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditDictionary(ctx, two.User.ID, "pinyin", "", 0, &DictionaryEntry{Code: "jiu'ci", Word: "旧词", Weight: 10}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s})
	export := func(token string) []byte {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/users/me/dictionary/snapshot", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	snapshot := export(one.AccessToken)
	revision, err := s.RestoreDictionarySnapshot(ctx, two.User.ID, 1, bytes.NewReader(snapshot), cfg)
	if err != nil {
		t.Fatal("restore", err)
	}
	entries, _, err := s.DictionaryEntries(ctx, two.User.ID, "pinyin", "", 0, 200)
	if err != nil || len(entries) != 1 || entries[0].Word != "拟好" || entries[0].ID == created.Replacement.ID {
		t.Fatal("restored entries", entries, err)
	}
	positions, _, err := s.CandidatePositions(ctx, two.User.ID, "", 0, 200)
	if err != nil || len(positions) != 1 || positions[0].Position != 1 {
		t.Fatal("restored positions", positions, err)
	}
	raw, err := cfg.QuerySnapshot(ctx, map[string]any{"operation": "personal_query", "query": map[string]any{"operation": "candidates", "text": "nihao", "limit": 5}}, func(ctx context.Context, w io.Writer) error {
		return s.StreamDictionarySnapshot(ctx, two.User.ID, func(raw json.RawMessage) error { _, err := w.Write(append(raw, '\n')); return err })
	})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Candidates []struct {
			Word  string `json:"word"`
			Fixed int    `json:"fixed_position"`
		} `json:"candidates"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || len(result.Candidates) == 0 || result.Candidates[0].Word != "拟好" || result.Candidates[0].Fixed != 1 {
		t.Fatal("restored query", string(raw), err)
	}
	changes, _, err := s.DictionaryChanges(ctx, two.User.ID, 1, 200)
	if err != nil || len(changes) < 2 || !changes[0].Reset {
		t.Fatal("missing reset marker", changes, err)
	}
	before := export(two.AccessToken)
	if _, err = s.RestoreDictionarySnapshot(ctx, two.User.ID, revision, bytes.NewReader(snapshot[:len(snapshot)-10]), cfg); err == nil {
		t.Fatal("accepted truncated snapshot")
	}
	if !bytes.Equal(before, export(two.AccessToken)) {
		t.Fatal("failed restore changed state")
	}
	if _, err = s.RestoreDictionarySnapshot(ctx, two.User.ID, 1, bytes.NewReader(snapshot), cfg); err != errRevisionConflict {
		t.Fatal("stale restore", err)
	}
	// Force a database failure after target deletion and entry insertion to prove
	// that replacement is atomic, not merely that corrupt inputs are rejected early.
	if _, err = s.pool.Exec(ctx, `CREATE FUNCTION reject_restore_position() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''test restore failure''; END'; CREATE TRIGGER reject_restore_position BEFORE INSERT ON user_candidate_positions FOR EACH ROW EXECUTE FUNCTION reject_restore_position()`); err != nil {
		t.Fatal(err)
	}
	_, restoreErr := s.RestoreDictionarySnapshot(ctx, two.User.ID, revision, bytes.NewReader(snapshot), cfg)
	if _, err = s.pool.Exec(ctx, `DROP TRIGGER reject_restore_position ON user_candidate_positions; DROP FUNCTION reject_restore_position()`); err != nil {
		t.Fatal(err)
	}
	if restoreErr == nil || !bytes.Equal(before, export(two.AccessToken)) {
		t.Fatal("database failure did not roll back replacement", restoreErr)
	}
	// Delete only the derived target overlay; rebuilding must use the latest reset
	// boundary and must not resurrect the target's pre-restore word.
	if _, err = s.pool.Exec(ctx, `DELETE FROM user_dictionary_overlay WHERE user_id=$1`, two.User.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, export(two.AccessToken)) {
		t.Fatal("restored baseline rebuild differs")
	}
	// A valid empty snapshot explicitly replaces all dictionary state.
	empty := signedSnapshot(`{"type":"header","format":"msime-dictionary-snapshot","version":1,"revision":0}`)
	if _, err = s.RestoreDictionarySnapshot(ctx, two.User.ID, revision, bytes.NewReader(empty), cfg); err != nil {
		t.Fatal("empty restore", err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM user_dictionary_overlay WHERE user_id=$1`, two.User.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("empty restore resurrected data", count, err)
	}
	if !bytes.Equal(snapshot, export(one.AccessToken)) {
		t.Fatal("source snapshot changed")
	}
}
