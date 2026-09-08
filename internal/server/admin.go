package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	adminweb "github.com/metasequoiaime/MSIME-Backend/admin-web"
)

type AdminConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	TokenEnv string `json:"token_env"`
	token    string
}

func (c *AdminConfig) validate(authEnabled bool, clients []Client) error {
	if !c.Enabled {
		return nil
	}
	if c.Host == "" {
		c.Host = "admin.msime.app"
	}
	if c.TokenEnv == "" {
		c.TokenEnv = "MSIME_ADMIN_TOKEN"
	}
	c.token = os.Getenv(c.TokenEnv)
	if !authEnabled {
		return errors.New("admin requires auth.enabled and PostgreSQL")
	}
	if len(c.Host) > 253 || strings.ContainsAny(c.Host, "/:?#@ \\\r\n\t") || !strings.Contains(c.Host, ".") {
		return errors.New("invalid admin host: use a hostname without port")
	}
	if len(c.token) < 32 || strings.ContainsAny(c.token, " \r\n\t") {
		return errors.New("admin token requires at least 32 non-whitespace bytes")
	}
	for _, client := range clients {
		if c.token == os.Getenv(client.TokenEnv) {
			return errors.New("admin token must differ from client tokens")
		}
	}
	return nil
}

var adminAssets = adminweb.Handler()

func (s *Server) serveAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.config.Admin.Enabled {
		return false
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	if !strings.EqualFold(host, s.config.Admin.Host) {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if origin := r.Header.Get("Origin"); origin != "" && origin != "https://"+r.Host && !(r.TLS == nil && origin == "http://"+r.Host) {
		fail(w, 403, "origin_denied")
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		auth := r.Header.Get("Authorization")
		supplied := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
		expected := sha256.Sum256([]byte(s.config.Admin.token))
		if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare(supplied[:], expected[:]) != 1 {
			fail(w, 401, "unauthorized")
			return true
		}
		if !s.allow(Client{ID: "admin", RequestsPerMinute: 120}, time.Now()) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limit_exceeded")
			return true
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		s.accounts.AdminHTTP(w, r.WithContext(ctx))
		return true
	}
	if !adminweb.IsPath(r.URL.Path) {
		http.NotFound(w, r)
		return true
	}
	adminAssets.ServeHTTP(w, r)
	return true
}
