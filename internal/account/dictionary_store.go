package account

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type DictionaryEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Code      string    `json:"code"`
	Word      string    `json:"word"`
	Weight    int64     `json:"weight"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}
type DictionaryChange struct {
	Revision    int64            `json:"revision"`
	Previous    *DictionaryEntry `json:"previous"`
	Replacement *DictionaryEntry `json:"replacement"`
}

var errDictionaryDuplicate = errors.New("dictionary_duplicate")
var errDictionaryLimit = errors.New("dictionary_limit")

func (s *Store) DictionaryEntries(ctx context.Context, user, kind, search string, offset, limit int) ([]DictionaryEntry, bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,kind,code,word,weight,revision,updated_at FROM user_dictionary_entries WHERE user_id=$1 AND kind=$2 AND ($3='' OR strpos(lower(code),lower($3))>0 OR strpos(lower(word),lower($3))>0) ORDER BY code,word,id LIMIT $4 OFFSET $5`, user, kind, search, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []DictionaryEntry{}
	for rows.Next() {
		var e DictionaryEntry
		if err = rows.Scan(&e.ID, &e.Kind, &e.Code, &e.Word, &e.Weight, &e.Revision, &e.UpdatedAt); err != nil {
			return nil, false, err
		}
		out = append(out, e)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}
func (s *Store) dictionaryEdit(ctx context.Context, tx pgx.Tx, user, kind, id string, expected int64, next *DictionaryEntry) (DictionaryChange, error) {
	var previous *DictionaryEntry
	if id != "" {
		previous = &DictionaryEntry{}
		err := tx.QueryRow(ctx, `SELECT id,kind,code,word,weight,revision,updated_at FROM user_dictionary_entries WHERE user_id=$1 AND kind=$2 AND id=$3`, user, kind, id).Scan(&previous.ID, &previous.Kind, &previous.Code, &previous.Word, &previous.Weight, &previous.Revision, &previous.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return DictionaryChange{}, errDataNotFound
		}
		if err != nil {
			return DictionaryChange{}, err
		}
		if previous.Revision != expected {
			return DictionaryChange{}, errRevisionConflict
		}
	}
	if next != nil {
		var duplicate bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_dictionary_entries WHERE user_id=$1 AND kind=$2 AND code=$3 AND word=$4 AND id<>$5)`, user, kind, next.Code, next.Word, id).Scan(&duplicate); err != nil {
			return DictionaryChange{}, err
		}
		if duplicate {
			return DictionaryChange{}, errDictionaryDuplicate
		}
		if id == "" {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_dictionary_entries WHERE user_id=$1`, user).Scan(&count); err != nil {
				return DictionaryChange{}, err
			}
			if count >= 100000 {
				return DictionaryChange{}, errDictionaryLimit
			}
			id = randomToken()
		}
	}
	var revision int64
	err := tx.QueryRow(ctx, `INSERT INTO user_dictionary_state(user_id,revision) VALUES($1,1) ON CONFLICT(user_id) DO UPDATE SET revision=user_dictionary_state.revision+1 RETURNING revision`, user).Scan(&revision)
	if err != nil {
		return DictionaryChange{}, err
	}
	change := DictionaryChange{Revision: revision, Previous: previous}
	if next == nil {
		if _, err = tx.Exec(ctx, `DELETE FROM user_dictionary_entries WHERE user_id=$1 AND id=$2`, user, id); err != nil {
			return change, err
		}
	} else {
		replacement := *next
		replacement.ID = id
		replacement.Kind = kind
		replacement.Revision = revision
		err = tx.QueryRow(ctx, `INSERT INTO user_dictionary_entries(id,user_id,kind,code,word,weight,revision) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET code=excluded.code,word=excluded.word,weight=excluded.weight,revision=excluded.revision,updated_at=now() RETURNING updated_at`, id, user, kind, replacement.Code, replacement.Word, replacement.Weight, revision).Scan(&replacement.UpdatedAt)
		if err != nil {
			return change, err
		}
		change.Replacement = &replacement
	}
	raw, err := json.Marshal(change)
	if err != nil {
		return change, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_dictionary_changes(user_id,revision,change) VALUES($1,$2,$3::jsonb)`, user, revision, raw)
	return change, err
}
func (s *Store) EditDictionary(ctx context.Context, user, kind, id string, expected int64, next *DictionaryEntry) (DictionaryChange, error) {
	tx, err := s.userDataTransaction(ctx, user)
	if err != nil {
		return DictionaryChange{}, err
	}
	defer tx.Rollback(ctx)
	change, err := s.dictionaryEdit(ctx, tx, user, kind, id, expected, next)
	if err != nil {
		return change, err
	}
	return change, tx.Commit(ctx)
}
func (s *Store) ImportDictionary(ctx context.Context, user, kind string, entries []DictionaryEntry) (int64, error) {
	tx, err := s.userDataTransaction(ctx, user)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	for i := range entries {
		change, err := s.dictionaryEdit(ctx, tx, user, kind, "", 0, &entries[i])
		if err != nil {
			return 0, err
		}
		revision = change.Revision
	}
	return revision, tx.Commit(ctx)
}
func (s *Store) DictionaryChanges(ctx context.Context, user string, after int64, limit int) ([]DictionaryChange, bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT change FROM user_dictionary_changes WHERE user_id=$1 AND revision>$2 ORDER BY revision LIMIT $3`, user, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []DictionaryChange{}
	for rows.Next() {
		var raw []byte
		var c DictionaryChange
		if err = rows.Scan(&raw); err != nil {
			return nil, false, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, false, err
		}
		result = append(result, c)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > limit
	if more {
		result = result[:limit]
	}
	return result, more, nil
}

// StreamDictionary exports one database snapshot without retaining the user's entire dictionary in memory.
func (s *Store) StreamDictionary(ctx context.Context, user, kind string, emit func(DictionaryEntry) error) error {
	rows, err := s.pool.Query(ctx, `SELECT id,kind,code,word,weight,revision,updated_at FROM user_dictionary_entries WHERE user_id=$1 AND kind=$2 ORDER BY code,word,id`, user, kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e DictionaryEntry
		if err = rows.Scan(&e.ID, &e.Kind, &e.Code, &e.Word, &e.Weight, &e.Revision, &e.UpdatedAt); err != nil {
			return err
		}
		if err = emit(e); err != nil {
			return err
		}
	}
	return rows.Err()
}
