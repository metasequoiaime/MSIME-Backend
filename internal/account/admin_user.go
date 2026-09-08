package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// adminUser returns only profile and session metadata, never identity subjects or credentials.
func (a *Service) adminUser(w http.ResponseWriter, r *http.Request, id string) {
	if !resourceText(id, 1, 128, false) || strings.Contains(id, "/") {
		writeError(w, 400, "invalid_id")
		return
	}
	var result json.RawMessage
	err := a.store.pool.QueryRow(r.Context(), `SELECT json_build_object(
 'id',u.id,'display_name',u.display_name,'created_at',u.created_at,
 'providers',COALESCE((SELECT json_agg(DISTINCT provider ORDER BY provider) FROM auth_identities WHERE user_id=u.id),'[]'::json),
 'active_sessions',(SELECT count(*) FROM auth_sessions WHERE user_id=u.id AND NOT revoked AND expires_at>now()),
 'total_sessions',(SELECT count(*) FROM auth_sessions WHERE user_id=u.id),
 'skins',(SELECT count(*) FROM community_skins WHERE owner_id=u.id),
 'dictionaries',(SELECT count(*) FROM community_resources WHERE owner_id=u.id AND kind='dictionary'),
 'replies',(SELECT count(*) FROM community_resources WHERE owner_id=u.id AND kind='reply'),
 'sessions',COALESCE((SELECT json_agg(x ORDER BY created_at DESC,id DESC) FROM (
 SELECT id,created_at,expires_at,CASE WHEN revoked THEN 'revoked' WHEN expires_at<=now() THEN 'expired' ELSE 'active' END AS status
 FROM auth_sessions WHERE user_id=u.id ORDER BY created_at DESC,id DESC LIMIT 50
 ) x),'[]'::json)) FROM auth_users u WHERE u.id=$1`, id).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found")
		return
	}
	if err != nil {
		a.error(w, err)
		return
	}
	write(w, 200, result)
}
