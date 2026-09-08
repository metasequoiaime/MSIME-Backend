package server

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestInputValidationAndDisabledEngine(t *testing.T) {
	s := fixture(t, nil)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"unicode", `{"text":"4e2d"}`, 503},
		{"unicode", `{"text":"xyz"}`, 400},
		{"unicode", `{"text":"4e2d","resources":"/tmp"}`, 400},
		{"unicode", `{"text":"4e2d","scheme":"wubi"}`, 400},
		{"datetime", `{"text":"date","timezone":"invalid/zone"}`, 400},
		{"datetime", `{"text":"date","time":"yesterday"}`, 400},
		{"english", `{"text":"你好"}`, 400},
		{"candidates", `{"text":"nihao","scheme":"unknown"}`, 400},
		{"gloss", `{"text":"hello","direction":"en-fr"}`, 400},
		{"helpcode", `{"text":"你好","schema":"../secret"}`, 400},
		{"unknown", `{"text":"hello"}`, 404},
	} {
		w := call(s, "POST", "/v1/input/"+tc.path, tc.body)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
func TestRealNativeHTTP(t *testing.T) {
	binary := os.Getenv("MSIME_ENGINE_TEST_BINARY")
	if binary == "" {
		t.Skip("需要真实原生 Engine 二进制")
	}
	s := fixture(t, nil)
	s.config.Engine.Binary = binary
	for _, tc := range []struct{ path, body, fragment string }{
		{"unicode", `{"text":"4E2D","limit":5}`, "中"},
		{"datetime", `{"text":"date","time":"2026-09-08T18:00:00Z","timezone":"Asia/Shanghai","limit":5}`, "2026-09-09"},
		{"segmentation", `{"text":"nihao"}`, "ni'hao"},
		{"segmentation", `{"text":"nihc","scheme":"shuangpin","profile":"xiaohe"}`, "ni'hao"},
	} {
		w := call(s, "POST", "/v1/input/"+tc.path, tc.body)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		var payload map[string]any
		if json.Unmarshal(w.Body.Bytes(), &payload) != nil {
			t.Fatal(w.Body.String())
		}
		raw, _ := json.Marshal(payload)
		if !bytes.Contains(raw, []byte(tc.fragment)) {
			t.Fatalf("missing %s: %s", tc.fragment, raw)
		}
	}
}

func TestPublishedDictionaryHTTP(t *testing.T) {
	binary, resources := os.Getenv("MSIME_ENGINE_TEST_BINARY"), os.Getenv("MSIME_ENGINE_TEST_RESOURCES")
	if binary == "" || resources == "" {
		t.Skip("需要原生 Engine 及校验后的发布词库")
	}
	s := fixture(t, nil)
	s.config.Engine.Binary = binary
	s.config.Engine.Resources = resources
	for _, tc := range []struct{ op, body, word string }{
		{"candidates", `{"text":"nihao","limit":5}`, "你好"},
		{"candidates", `{"text":"nihc","scheme":"shuangpin","limit":5}`, "你好"},
		{"candidates", `{"text":"wq","scheme":"wubi","limit":5}`, "你"},
		{"english", `{"text":"HELLO","limit":5}`, "hello"},
		{"gloss", `{"text":"hello"}`, "喂"},
		{"convert", `{"text":"头发发展，后台学习"}`, "頭髮發展，後臺學習"},
		{"annotate", `{"text":"重庆银行"}`, "chong'qing'yin'hang"},
		{"jianpin", `{"text":"zg","limit":5}`, "中国"},
		{"emoji", `{"text":"kaixin","limit":5}`, "😄"},
		{"kaomoji", `{"text":"kaixin","limit":5}`, "candidates"},
	} {
		w := call(s, "POST", "/v1/input/"+tc.op, tc.body)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(tc.word)) {
			t.Fatalf("%s: %d %s", tc.op, w.Code, w.Body.String())
		}
	}
	for _, kind := range []string{"emoji", "kaomoji", "symbols"} {
		first := call(s, "GET", "/v1/catalog/"+kind+"?limit=2", "")
		var page struct {
			Items      []struct{ Text string }
			Categories []any
			HasMore    bool `json:"has_more"`
		}
		if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &page) != nil || len(page.Items) != 2 || !page.HasMore || len(page.Categories) == 0 {
			t.Fatalf("catalog %s: %s", kind, first.Body.String())
		}
		second := call(s, "GET", "/v1/catalog/"+kind+"?limit=2&offset=2", "")
		if second.Code != 200 || bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
			t.Fatal("分页未前进", kind)
		}
	}
	w := call(s, "GET", "/v1/catalog/symbols?limit=201", "")
	if w.Code != 400 {
		t.Fatal("目录未限制页大小")
	}
}
