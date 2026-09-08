package account

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type AdminIdentity struct {
	Subject string `json:"subject"`
	Email   string `json:"email"`
}
type AdminLoginFlow struct{ Nonce, Verifier string }
type adminActorKey struct{}

func WithAdminActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, adminActorKey{}, actor)
}
func adminActor(ctx context.Context) string {
	value, _ := ctx.Value(adminActorKey{}).(string)
	if value == "" {
		return "legacy-token"
	}
	return value
}
func (a *Service) SaveAdminFlow(ctx context.Context, state string, flow AdminLoginFlow) error {
	_, err := a.store.pool.Exec(ctx, `INSERT INTO admin_login_flows(state_hash,nonce,verifier) VALUES($1,$2,$3)`, hash(state), flow.Nonce, flow.Verifier)
	return err
}
func (a *Service) ConsumeAdminFlow(ctx context.Context, state string) (AdminLoginFlow, error) {
	var flow AdminLoginFlow
	err := a.store.pool.QueryRow(ctx, `DELETE FROM admin_login_flows WHERE state_hash=$1 AND expires_at>now() RETURNING nonce,verifier`, hash(state)).Scan(&flow.Nonce, &flow.Verifier)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrInvalid
	}
	return flow, err
}
func (a *Service) CreateAdminSession(ctx context.Context, identity AdminIdentity) (string, error) {
	token := randomToken()
	tx, err := a.store.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT enabled FROM admin_members WHERE email=$1 FOR UPDATE`, strings.ToLower(identity.Email)).Scan(&enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err == nil && !enabled {
		return "", ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_sessions(token_hash,subject,email) VALUES($1,$2,$3)`, hash(token), identity.Subject, strings.ToLower(identity.Email))
	if err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}
func (a *Service) AdminSession(ctx context.Context, token string) (AdminIdentity, error) {
	var identity AdminIdentity
	if len(token) != 64 {
		return identity, ErrInvalid
	}
	err := a.store.pool.QueryRow(ctx, `SELECT subject,email FROM admin_sessions WHERE token_hash=$1 AND expires_at>now()`, hash(token)).Scan(&identity.Subject, &identity.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrInvalid
	}
	return identity, err
}
func (a *Service) DeleteAdminSession(ctx context.Context, token string) error {
	_, err := a.store.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, hash(token))
	return err
}
