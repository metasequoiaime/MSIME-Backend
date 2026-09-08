package account

import (
	"testing"
	"time"
)

func TestPruneRemovesOnlyExpiredAuthenticationData(t *testing.T) {
	db := testStore(t)
	a := &Service{store: db}
	ctx := t.Context()
	if _, err := db.pool.Exec(ctx, `TRUNCATE admin_login_flows,admin_sessions`); err != nil {
		t.Fatal(err)
	}
	active := complete(t, db, Identity{"email", "active@example.test"})
	expired := complete(t, db, Identity{"email", "expired@example.test"})
	if _, err := db.pool.Exec(ctx, `UPDATE auth_sessions SET expires_at=now()-interval '1 minute' WHERE user_id=$1`, expired.User.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"active", "expired"} {
		if err := db.PutChallenge(ctx, Challenge{IDHash: id, Provider: "google", Nonce: id}); err != nil {
			t.Fatal(err)
		}
		if err := db.Rate(ctx, id, 10, time.Minute); err != nil {
			t.Fatal(err)
		}
		if err := a.SaveAdminFlow(ctx, id, AdminLoginFlow{Nonce: id, Verifier: id}); err != nil {
			t.Fatal(err)
		}
		token, err := a.CreateAdminSession(ctx, AdminIdentity{Subject: id, Email: id + "@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		if id == "expired" {
			for _, statement := range []struct{ sql, key string }{
				{`UPDATE auth_challenges SET expires_at=now()-interval '1 minute' WHERE id_hash=$1`, id},
				{`UPDATE auth_rates SET expires_at=now()-interval '1 minute' WHERE key=$1`, id},
				{`UPDATE admin_login_flows SET expires_at=now()-interval '1 minute' WHERE state_hash=$1`, hash(id)},
				{`UPDATE admin_sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=$1`, hash(token)},
			} {
				if _, err := db.pool.Exec(ctx, statement.sql, statement.key); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	db.Prune(ctx)
	for _, table := range []string{"auth_challenges", "auth_rates", "auth_sessions", "admin_login_flows", "admin_sessions"} {
		var n int
		if err := db.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE expires_at<now()`).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	if _, err := db.Authenticate(ctx, active.AccessToken); err != nil {
		t.Fatal("active session pruned", err)
	}
	if _, err := db.Authenticate(ctx, expired.AccessToken); err != ErrInvalid {
		t.Fatal("expired session retained", err)
	}
	for _, q := range []string{`SELECT count(*) FROM auth_challenges WHERE id_hash='active'`, `SELECT count(*) FROM auth_rates WHERE key='active'`, `SELECT count(*) FROM admin_sessions WHERE subject='active'`} {
		var n int
		if err := db.pool.QueryRow(ctx, q).Scan(&n); err != nil || n != 1 {
			t.Fatal("active record pruned", n, err)
		}
	}
	if _, err := a.ConsumeAdminFlow(ctx, "active"); err != nil {
		t.Fatal("active flow pruned", err)
	}
	if _, _, err := db.Me(ctx, expired.User.ID); err != nil {
		t.Fatal("prune removed account", err)
	}
	db.Prune(ctx)
}
