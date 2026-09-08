package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

// Opt-in capacity check; uses the existing synthetic PostgreSQL test database.
func TestSnapshotRestoreAtEntryLimit(t *testing.T) {
	if os.Getenv("MSIME_TEST_LARGE_RESTORE") != "1" {
		t.Skip("显式开启十万词条容量验证")
	}
	cfg := engine.Config{Binary: os.Getenv("MSIME_ENGINE_TEST_BINARY"), Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}
	if cfg.Binary == "" || cfg.Resources == "" {
		t.Fatal("需要真实 Engine 与发布词库")
	}
	s := testStore(t)
	user := complete(t, s, Identity{"email", "capacity@example.test"})
	file, err := os.CreateTemp(t.TempDir(), "snapshot-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	encoder := json.NewEncoder(io.MultiWriter(file, hash))
	records := 0
	emit := func(v any) {
		t.Helper()
		if err := encoder.Encode(v); err != nil {
			t.Fatal(err)
		}
		records++
	}
	const count = 100000
	emit(map[string]any{"type": "header", "format": "msime-dictionary-snapshot", "version": 1, "revision": count})
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	for _, kind := range []string{"entry", "overlay"} {
		for i := 0; i < count; i++ {
			n := i
			letters := [4]byte{}
			for j := 3; j >= 0; j-- {
				letters[j] = byte('a' + n%26)
				n /= 26
			}
			word := "msime" + string(letters[:])
			e := DictionaryEntry{ID: fmt.Sprintf("synthetic-%d", i), Kind: "english", Code: word, Word: word, Weight: 10, Revision: int64(i + 1), UpdatedAt: now}
			record := map[string]any{"type": kind, "data": e}
			if kind == "overlay" {
				record["deleted"] = false
			}
			emit(record)
		}
	}
	if err = json.NewEncoder(file).Encode(map[string]any{"type": "footer", "records": records, "sha256": hex.EncodeToString(hash.Sum(nil))}); err != nil {
		t.Fatal(err)
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), snapshotRestoreTimeout)
	defer cancel()
	start := time.Now()
	revision, err := s.RestoreDictionarySnapshot(ctx, user.User.ID, 0, file, cfg)
	t.Logf("snapshot bytes=%d restore=%s revision=%d", size, time.Since(start), revision)
	if err != nil {
		t.Fatal(err)
	}
	var entries, overlays int
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM user_dictionary_entries WHERE user_id=$1),(SELECT count(*) FROM user_dictionary_overlay WHERE user_id=$1)`, user.User.ID).Scan(&entries, &overlays); err != nil || entries != count || overlays != count {
		t.Fatal(entries, overlays, err)
	}
	start = time.Now()
	exported := 0
	if err = s.StreamFullDictionarySnapshot(ctx, user.User.ID, func(json.RawMessage) error { exported++; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Logf("export=%s records=%d", time.Since(start), exported)
	if exported != 1+2*count {
		t.Fatal(exported)
	}
	start = time.Now()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("migration on populated database=%s", time.Since(start))
	start = time.Now()
	raw, err := cfg.QuerySnapshot(ctx, map[string]any{"operation": "personal_query", "query": map[string]any{"operation": "english", "text": "msimeaaaa", "limit": 5}}, func(ctx context.Context, w io.Writer) error {
		return s.StreamDictionarySnapshot(ctx, user.User.ID, func(raw json.RawMessage) error { _, err := w.Write(append(raw, '\n')); return err })
	})
	t.Logf("personal query=%s", time.Since(start))
	if err != nil {
		t.Fatal("restored dictionary is not queryable at its supported size", err)
	}
	var result struct {
		Candidates []struct {
			Word string `json:"word"`
		} `json:"candidates"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || len(result.Candidates) == 0 || result.Candidates[0].Word != "msimeaaaa" {
		t.Fatal("capacity query", string(raw), err)
	}

}
