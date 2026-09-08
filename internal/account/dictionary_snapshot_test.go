package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFullDictionarySnapshotHTTP(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "snapshot-one@example.test"})
	two := complete(t, s, Identity{"email", "snapshot-two@example.test"})
	entry, err := s.EditDictionary(ctx, one.User.ID, "pinyin", "", 0, &DictionaryEntry{Code: "ni'hao", Word: "你好", Weight: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditDictionary(ctx, one.User.ID, "pinyin", entry.Replacement.ID, entry.Revision, &DictionaryEntry{Code: "ni'hao", Word: "拟好", Weight: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetCandidatePosition(ctx, one.User.ID, 2, CandidatePosition{Context: "ni'hao", Code: "ni'hao", Word: "拟好", Position: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO user_candidate_selections(user_id,context,code,word,count) VALUES($1,'ni','ni','你',1)`, one.User.ID); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s})
	export := func(token string, status int) []map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/users/me/dictionary/snapshot", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("snapshot: %d %s", w.Code, w.Body.String())
		}
		if status != 200 {
			return nil
		}
		if w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("snapshot headers", w.Header())
		}
		if err := decodeDictionarySnapshot(bytes.NewReader(w.Body.Bytes()), func(snapshotRecord) error { return nil }); err != nil {
			t.Fatal("export not accepted by restore decoder", err)
		}
		lines := bytes.Split(bytes.TrimSuffix(w.Body.Bytes(), []byte{'\n'}), []byte{'\n'})
		records := []map[string]any{}
		hash := sha256.New()
		for i, line := range lines {
			var v map[string]any
			if err := json.Unmarshal(line, &v); err != nil {
				t.Fatal(err)
			}
			if i == len(lines)-1 {
				if v["type"] != "footer" || v["records"] != float64(i) || v["sha256"] != hex.EncodeToString(hash.Sum(nil)) {
					t.Fatal("invalid completion checksum", v)
				}
			} else {
				hash.Write(line)
				hash.Write([]byte{'\n'})
				records = append(records, v)
			}
		}
		if bytes.Contains(w.Body.Bytes(), []byte(one.User.ID)) {
			t.Fatal("source account leaked into portable snapshot")
		}
		return records
	}
	export("device-token", 401)
	records := export(one.AccessToken, 200)
	counts := map[string]int{}
	deleted := 0
	for _, r := range records {
		counts[r["type"].(string)]++
		if r["deleted"] == true {
			deleted++
		}
	}
	if counts["header"] != 1 || counts["entry"] != 1 || counts["overlay"] != 2 || counts["position"] != 1 || counts["selection"] != 1 || deleted != 1 {
		t.Fatal("incomplete state", counts, deleted)
	}
	if records[0]["revision"] != float64(3) || records[0]["format"] != "msime-dictionary-snapshot" {
		t.Fatal(records[0])
	}
	isolated := export(two.AccessToken, 200)
	if len(isolated) != 1 || isolated[0]["revision"] != float64(0) {
		t.Fatal("snapshot leak", isolated)
	}
}
