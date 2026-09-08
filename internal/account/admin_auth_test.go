package account

import (
	"context"
	"testing"
)

func TestAdminFlowsAndSessions(t *testing.T) {
	db := testStore(t)
	a := &Service{store: db}
	ctx := context.Background()
	state := randomToken()
	flow := AdminLoginFlow{Nonce: "nonce", Verifier: "verifier"}
	if err := a.SaveAdminFlow(ctx, state, flow); err != nil {
		t.Fatal(err)
	}
	result, err := a.ConsumeAdminFlow(ctx, state)
	if err != nil || result != flow {
		t.Fatal(result, err)
	}
	if _, err = a.ConsumeAdminFlow(ctx, state); err != ErrInvalid {
		t.Fatal("replay accepted", err)
	}
	if err = a.SaveAdminFlow(ctx, state, flow); err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, `UPDATE admin_login_flows SET expires_at=now()-interval '1 second' WHERE state_hash=$1`, hash(state)); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ConsumeAdminFlow(ctx, state); err != ErrInvalid {
		t.Fatal("expired state accepted")
	}
	identity := AdminIdentity{Subject: "google-123", Email: "admin@example.test"}
	token, err := a.CreateAdminSession(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = db.pool.QueryRow(ctx, `SELECT token_hash FROM admin_sessions WHERE token_hash=$1`, hash(token)).Scan(&stored); err != nil || stored == token {
		t.Fatal("raw token stored", err)
	}
	who, err := a.AdminSession(ctx, token)
	if err != nil || who != identity {
		t.Fatal(who, err)
	}
	if _, err = db.Authenticate(ctx, token); err != ErrInvalid {
		t.Fatal("admin session accepted as user")
	}
	if err = a.DeleteAdminSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = a.AdminSession(ctx, token); err != ErrInvalid {
		t.Fatal("logout failed")
	}
	token, err = a.CreateAdminSession(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, `UPDATE admin_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hash(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = a.AdminSession(ctx, token); err != ErrInvalid {
		t.Fatal("expired session accepted")
	}
}
