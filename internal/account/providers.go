package account

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}
type audienceVerifiers []Verifier

func (v audienceVerifiers) Verify(ctx context.Context, raw string) (*oidc.IDToken, error) {
	for _, verifier := range v {
		token, err := verifier.Verify(ctx, raw)
		if err == nil {
			return token, nil
		}
	}
	return nil, ErrInvalid
}

type Sender interface {
	Send(context.Context, string, string, string) error
}

func makeVerifiers(ctx context.Context, c Config, client *http.Client) map[string]Verifier {
	result := map[string]Verifier{}
	for name, v := range map[string]struct {
		ids          []string
		issuer, keys string
	}{
		"google": {c.Google.ClientIDs, "https://accounts.google.com", "https://www.googleapis.com/oauth2/v3/certs"},
		"apple":  {c.Apple.ClientIDs, "https://appleid.apple.com", "https://appleid.apple.com/auth/keys"},
	} {
		if len(v.ids) == 0 {
			continue
		}
		keys := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), v.keys)
		verifiers := audienceVerifiers{}
		for _, id := range v.ids {
			verifiers = append(verifiers, oidc.NewVerifier(v.issuer, keys, &oidc.Config{ClientID: id, SupportedSigningAlgs: []string{"RS256"}}))
		}
		result[name] = verifiers
	}
	return result
}
func (a *Service) identity(ctx context.Context, c Challenge, credential string) (Identity, error) {
	if v := a.verifiers[c.Provider]; v != nil {
		token, e := v.Verify(ctx, credential)
		if e != nil || token.Subject == "" || len(token.Subject) > 255 || token.IssuedAt.IsZero() || token.IssuedAt.After(time.Now().Add(30*time.Second)) || subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(c.Nonce)) != 1 {
			return Identity{}, ErrInvalid
		}
		return Identity{c.Provider, token.Subject}, nil
	}
	if c.Provider == "wechat" {
		values := url.Values{"appid": {a.config.Wechat.AppID}, "secret": {os.Getenv(a.config.Wechat.SecretEnv)}, "code": {credential}, "grant_type": {"authorization_code"}}
		// 微信仅提供 query 参数换码接口；不记录请求 URL 或供应商响应。
		req, e := http.NewRequestWithContext(ctx, "GET", "https://api.weixin.qq.com/sns/oauth2/access_token?"+values.Encode(), nil)
		if e != nil {
			return Identity{}, ErrInvalid
		}
		res, e := a.client.Do(req)
		if e != nil {
			return Identity{}, errors.New("微信上游不可用")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return Identity{}, ErrInvalid
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, 65537))
		if e != nil || len(b) > 65536 {
			return Identity{}, ErrInvalid
		}
		var v struct {
			OpenID      string `json:"openid"`
			AccessToken string `json:"access_token"`
			ErrorCode   int    `json:"errcode"`
		}
		if json.Unmarshal(b, &v) != nil || v.ErrorCode != 0 || v.OpenID == "" || len(v.OpenID) > 255 || v.AccessToken == "" {
			return Identity{}, ErrInvalid
		}
		return Identity{"wechat", a.config.Wechat.AppID + ":" + v.OpenID}, nil
	}
	return Identity{}, ErrInvalid
}
