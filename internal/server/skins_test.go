package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkinHTTP(t *testing.T) {
	s := fixture(t, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/v1/skins", nil))
	if w.Code != 401 {
		t.Fatal("skin auth", w.Code)
	}
	w = call(s, "GET", "/v1/skins?layout=horizontal&theme=dark", "")
	var result struct {
		Skins []json.RawMessage `json:"skins"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Skins) != 4 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, p := range []string{"/v1/skins/fluent", "/v1/skins/source", "/v1/skins/license"} {
		if w := call(s, "GET", p, ""); w.Code != 200 {
			t.Fatal(p, w.Code)
		}
	}
	css := call(s, "GET", "/v1/skins/fluent/resources/horizontal_dark.css", "")
	if css.Code != 200 || !strings.HasPrefix(css.Header().Get("Content-Type"), "text/css") || css.Header().Get("Content-Security-Policy") == "" {
		t.Fatal(css.Code, css.Header())
	}
	for _, p := range []string{"/v1/skins/no-such-skin", "/v1/skins/fluent/resources/config.json", "/v1/skins/fluent/resources/%2e%2e%2fLICENSE"} {
		w := call(s, "GET", p, "")
		if w.Code != 404 {
			t.Fatal(p, w.Code)
		}
	}
	for _, p := range []string{"/v1/skins?layout=unknown", "/v1/skins?theme=unknown", "/v1/skins?path=/etc"} {
		if w := call(s, "GET", p, ""); w.Code != 400 {
			t.Fatal(p, w.Code)
		}
	}
}
