package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/metasequoiaime/MSIME-Backend/internal/engine"
)

func TestWindowsDictionaryTextRoundTrip(t *testing.T) {
	cfg := engine.Config{Binary: os.Getenv("MSIME_ENGINE_TEST_BINARY"), Resources: os.Getenv("MSIME_ENGINE_TEST_RESOURCES")}
	if cfg.Binary == "" || cfg.Resources == "" {
		t.Skip("需要真实 Engine 与发布词库")
	}
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "windows-format-one@example.test"})
	two := complete(t, s, Identity{"email", "windows-format-two@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s, engine: cfg})
	call := func(method, path, token string, body any, status int) string {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatal(method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, tc := range []struct{ kind, windows, standard string }{
		{"pinyin", "拟蒿\tni'hao\t10\n", "拟蒿\tni'hao\t10\n"},
		{"wubi", "测试词\tabcd\t10\n", "测试词\tabcd\t10\n"},
		{"english", "zzwindows\tZzwindows\t10\n", "Zzwindows\tzzwindows\t10\n"},
		{"quick", "zzwindows\t测试短语\t10\n", "测试短语\tzzwindows\t10\n"},
	} {
		path := "/v1/users/me/dictionaries/" + tc.kind
		call("POST", path+"/import", one.AccessToken, map[string]any{"text": tc.windows, "format": "windows"}, 200)
		windows := call("GET", path+"/export?format=windows", one.AccessToken, nil, 200)
		standard := call("GET", path+"/export", one.AccessToken, nil, 200)
		if windows != tc.windows || standard != tc.standard {
			t.Fatal("column order", tc.kind, windows, standard)
		}
		call("POST", path+"/import", two.AccessToken, map[string]any{"text": windows, "format": "windows"}, 200)
		if out := call("GET", path+"/export?format=windows", two.AccessToken, nil, 200); out != windows {
			t.Fatal("round trip", tc.kind, out)
		}
	}
	created, err := s.EditDictionary(ctx, one.User.ID, "pinyin", "", 0, &DictionaryEntry{Code: "ni", Word: "你", Weight: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditManagedDictionary(ctx, one.User.ID, created.Revision, DictionaryEntry{Kind: "pinyin", Code: "ni'hao", Word: "你好"}, &DictionaryEntry{Kind: "pinyin", Code: "ni'hao", Word: "你好", Weight: 123}, cfg); err != nil {
		t.Fatal(err)
	}
	windows := call("GET", "/v1/users/me/dictionaries/pinyin/export?format=windows", one.AccessToken, nil, 200)
	if strings.Contains(windows, "你\tni\t") || !strings.Contains(windows, "你好\tni'hao\t123\n") || !strings.Contains(windows, "拟蒿\tni'hao\t10\n") {
		t.Fatal("Windows pinyin selection", windows)
	}
	standard := call("GET", "/v1/users/me/dictionaries/pinyin/export", one.AccessToken, nil, 200)
	if !strings.Contains(standard, "你\tni\t10\n") || strings.Contains(standard, "你好\tni'hao\t123\n") {
		t.Fatal("standard format changed", standard)
	}
	call("GET", "/v1/users/me/dictionaries/quick/export?format=unknown", one.AccessToken, nil, 400)
	call("POST", "/v1/users/me/dictionaries/quick/import", one.AccessToken, map[string]any{"text": "x\ty\t10", "format": "unknown"}, 400)
}
