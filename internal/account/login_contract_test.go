package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestProviderLoginAndIdentityLinkHTTP(t *testing.T) {
	db := testStore(t)
	sender := &captureSender{}
	t.Setenv("LOGIN_TEST_PEPPER", strings.Repeat("p", 32))
	a := &Service{store: db, sender: sender, config: Config{PepperEnv: "LOGIN_TEST_PEPPER", Google: OIDCConfig{ClientIDs: []string{"client"}}, SMS: SMSConfig{TemplateCode: "test"}, Wechat: WechatConfig{AppID: "test-app", RedirectURI: "https://example.test/callback"}}}
	mux := http.NewServeMux()
	Mount(mux, a)
	owner := complete(t, db, Identity{"email", "owner@example.test"})
	other := complete(t, db, Identity{"email", "other@example.test"})
	challenge := func(provider, purpose, token string) map[string]string {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"provider": provider, "purpose": purpose})
		w := apiRequest(t, mux, "POST", "/v1/auth/challenges", string(raw), token, 201)
		var c struct {
			ID    string `json:"challenge_id"`
			Nonce string `json:"nonce"`
			URL   string `json:"authorization_url"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || len(c.ID) != 64 || len(c.Nonce) != 64 {
			t.Fatal(w.Body.String(), err)
		}
		return map[string]string{"id": c.ID, "nonce": c.Nonce, "url": c.URL}
	}
	login := func(c map[string]string, token string, status int) Tokens {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"challenge_id": c["id"], "credential": "signed-token"})
		w := apiRequest(t, mux, "POST", "/v1/auth/login", string(raw), token, status)
		var out Tokens
		if status == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.AccessToken == "" {
				t.Fatal("missing session", err)
			}
		}
		return out
	}
	bind := func(c map[string]string, subject string) {
		a.verifiers = map[string]Verifier{"google": contractVerifier(func(context.Context, string) (*oidc.IDToken, error) {
			return &oidc.IDToken{Subject: subject, Nonce: c["nonce"], IssuedAt: time.Now()}, nil
		})}
	}
	c := challenge("google", "login", "")
	bind(c, "google-person")
	google := login(c, "", 200)
	if google.User.ID == owner.User.ID {
		t.Fatal("unrelated identities merged")
	}
	login(c, "", 401)
	c = challenge("google", "link", owner.AccessToken)
	bind(c, "linked-person")
	login(c, "", 401)
	login(c, other.AccessToken, 401)
	linked := login(c, owner.AccessToken, 200)
	if linked.User.ID != owner.User.ID {
		t.Fatal("linked identity changed user")
	}
	_, ids, err := db.Me(t.Context(), owner.User.ID)
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	c = challenge("google", "link", owner.AccessToken)
	bind(c, "google-person")
	login(c, owner.AccessToken, 409)
	c = challenge("google", "login", "")
	bind(c, "valid-person")
	c["nonce"] = "wrong"
	login(c, "", 401)
	c = challenge("wechat", "login", "")
	u, err := url.Parse(c["url"])
	if err != nil || u.Host != "open.weixin.qq.com" || u.Query().Get("state") != c["id"] || u.Query().Get("appid") != "test-app" {
		t.Fatal(c, err)
	}
	apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"google","purpose":"bad"}`, "", 400)
	apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"google","purpose":"link"}`, "", 401)
	apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"phone","target":"invalid"}`, "", 400)
	w := apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"phone","target":"+12025550123"}`, "", 201)
	var phone struct {
		ID string `json:"challenge_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &phone); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"challenge_id": phone.ID, "credential": sender.code})
	apiRequest(t, mux, "POST", "/v1/auth/login", string(raw), "", 200)
	apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"phone","target":"+12025550123"}`, "", 429)
	if _, err := db.pool.Exec(t.Context(), `UPDATE auth_sessions SET created_at=now()-interval '15 minutes' WHERE user_id=$1`, owner.User.ID); err != nil {
		t.Fatal(err)
	}
	apiRequest(t, mux, "DELETE", "/v1/users/me", "", owner.AccessToken, 403)
	apiRequest(t, mux, "POST", "/v1/auth/challenges", `{"provider":"google","purpose":"link"}`, owner.AccessToken, 403)
	apiRequest(t, mux, "GET", "/v1/users/me", "", owner.AccessToken, 200)
}
