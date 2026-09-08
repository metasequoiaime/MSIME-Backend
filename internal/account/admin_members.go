package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
)

func (a *Service) AdminEmailAllowed(ctx context.Context, email string) (bool, error) {
	var allowed bool
	err := a.store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_members WHERE email=$1 AND enabled)`, strings.ToLower(email)).Scan(&allowed)
	return allowed, err
}

// AdminMembersHTTP is restricted to deployment-configured owners by the server gate.
func (a *Service) AdminMembersHTTP(w http.ResponseWriter, r *http.Request, owners []string) {
	if r.Method == "GET" {
		var result json.RawMessage
		err := a.store.pool.QueryRow(r.Context(), `SELECT COALESCE(json_agg(x ORDER BY email),'[]'::json) FROM (SELECT email,enabled,created_at,updated_at,(SELECT count(*) FROM admin_sessions s WHERE s.email=m.email AND expires_at>now()) AS sessions FROM admin_members m) x`).Scan(&result)
		if err != nil {
			a.error(w, err)
			return
		}
		write(w, 200, map[string]any{"owners": owners, "items": result})
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method_not_allowed")
		return
	}
	var v struct {
		Email  string `json:"email"`
		Action string `json:"action"`
	}
	if !read(w, r, &v) {
		return
	}
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	addr, err := mail.ParseAddress(v.Email)
	if err != nil || addr.Address != v.Email || len(v.Email) > 254 || (v.Action != "add" && v.Action != "enable" && v.Action != "disable" && v.Action != "revoke") {
		writeError(w, 400, "invalid_admin")
		return
	}
	for _, owner := range owners {
		if strings.EqualFold(owner, v.Email) {
			writeError(w, 403, "protected_owner")
			return
		}
	}
	tx, err := a.store.pool.Begin(r.Context())
	if err != nil {
		a.error(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize membership changes and the bounded account count.
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(73491203)`); err != nil {
		a.error(w, err)
		return
	}
	var affected int64
	if v.Action == "add" {
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM admin_members`).Scan(&count); err != nil {
			a.error(w, err)
			return
		}
		if count >= 100 {
			writeError(w, 409, "admin_limit")
			return
		}
		tag, e := tx.Exec(r.Context(), `INSERT INTO admin_members(email) VALUES($1) ON CONFLICT DO NOTHING`, v.Email)
		err = e
		affected = tag.RowsAffected()
	} else {
		tag, e := tx.Exec(r.Context(), `UPDATE admin_members SET enabled=CASE WHEN $2='revoke' THEN enabled ELSE $2='enable' END,updated_at=now() WHERE email=$1`, v.Email, v.Action)
		err = e
		affected = tag.RowsAffected()
	}
	if err != nil {
		a.error(w, err)
		return
	}
	if affected == 0 {
		if v.Action == "add" {
			writeError(w, 409, "admin_exists")
		} else {
			writeError(w, 404, "not_found")
		}
		return
	}
	// Also clear sessions on re-enable so historical credentials never regain access.
	if v.Action != "add" {
		if _, err = tx.Exec(r.Context(), `DELETE FROM admin_sessions WHERE email=$1`, v.Email); err != nil {
			a.error(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO admin_audit(action,target,actor) VALUES($1,$2,$3)`, "admin_"+v.Action, v.Email, adminActor(r.Context())); err != nil {
		a.error(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.error(w, err)
		return
	}
	write(w, 200, map[string]any{"ok": true, "affected": affected})
}
