package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// All records come from one SQL statement snapshot. Source user IDs and credentials
// never enter this portable format. The footer detects incomplete downloads.
func (s *Store) StreamFullDictionarySnapshot(ctx context.Context, user string, emit func(json.RawMessage) error) error {
	rows, err := s.pool.Query(ctx, `SELECT record FROM (
 SELECT 0 AS category,'' AS sort1,'' AS sort2,'' AS sort3,jsonb_build_object('type','header','format','msime-dictionary-snapshot','version',1,'revision',COALESCE((SELECT revision FROM user_dictionary_state WHERE user_id=$1),0)) AS record
 UNION ALL SELECT 1,kind,code,word,jsonb_build_object('type','entry','data',jsonb_build_object('id',id,'kind',kind,'code',code,'word',word,'weight',weight,'revision',revision,'updated_at',updated_at)) FROM user_dictionary_entries WHERE user_id=$1
 UNION ALL SELECT 2,kind,code,word,jsonb_build_object('type','overlay','deleted',deleted,'data',entry) FROM user_dictionary_overlay WHERE user_id=$1
 UNION ALL SELECT 3,context,code,word,jsonb_build_object('type','position','data',jsonb_build_object('context',context,'code',code,'word',word,'position',position)) FROM user_candidate_positions WHERE user_id=$1
 UNION ALL SELECT 4,context,code,word,jsonb_build_object('type','selection','data',jsonb_build_object('context',context,'code',code,'word',word,'count',count)) FROM user_candidate_selections WHERE user_id=$1
 ) snapshot ORDER BY category,sort1,sort2,sort3`, user)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if err = emit(raw); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (a *Service) dictionarySnapshot(w http.ResponseWriter, r *http.Request) {
	p, ok := a.principal(w, r, false)
	if !ok {
		return
	}
	started := false
	hash := sha256.New()
	records := 0
	err := a.store.StreamFullDictionarySnapshot(r.Context(), p.UserID, func(raw json.RawMessage) error {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Disposition", `attachment; filename="msime-dictionary-snapshot.ndjson"`)
			w.Header().Set("Cache-Control", "no-store")
			started = true
		}
		line := append(raw, '\n')
		hash.Write(line)
		records++
		_, err := w.Write(line)
		return err
	})
	if err != nil {
		if started {
			panic(http.ErrAbortHandler)
		}
		a.dictionaryError(w, err)
		return
	}
	footer, err := json.Marshal(map[string]any{"type": "footer", "records": records, "sha256": hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	if _, err = w.Write(append(footer, '\n')); err != nil {
		panic(http.ErrAbortHandler)
	}
}
