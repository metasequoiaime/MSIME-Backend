package account

import (
	"context"
	"testing"
)

func TestDefaultNamesAcrossRegistrationProfileRefreshAndRename(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tokens := complete(t, s, Identity{"apple", "default-name-fixture"})
	want := defaultUserName(tokens.User.ID)
	if tokens.User.DisplayName != want {
		t.Fatalf("registration nickname = %q", tokens.User.DisplayName)
	}
	// Simulate an account created before default names existed.
	if _, err := s.pool.Exec(ctx, "UPDATE auth_users SET display_name='' WHERE id=$1", tokens.User.ID); err != nil {
		t.Fatal(err)
	}
	user, _, err := s.Me(ctx, tokens.User.ID)
	if err != nil || user.DisplayName != want {
		t.Fatalf("legacy profile = %+v, %v", user, err)
	}
	refreshed, err := s.Refresh(ctx, tokens.RefreshToken)
	if err != nil || refreshed.User.DisplayName != want {
		t.Fatalf("refresh = %+v, %v", refreshed.User, err)
	}
	if err := s.UpdateName(ctx, user.ID, "我的昵称"); err != nil {
		t.Fatal(err)
	}
	user, _, err = s.Me(ctx, user.ID)
	if err != nil || user.DisplayName != "我的昵称" {
		t.Fatalf("rename = %+v, %v", user, err)
	}
	if err := s.UpdateName(ctx, user.ID, ""); err != nil {
		t.Fatal(err)
	}
	user, _, err = s.Me(ctx, user.ID)
	if err != nil || user.DisplayName != want {
		t.Fatalf("reset = %+v, %v", user, err)
	}
}
