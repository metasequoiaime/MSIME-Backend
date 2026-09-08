package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTencentSignature(t *testing.T) {
	payload := []byte(`{"Source":"auto","Target":"en","ProjectId":0,"SourceTextList":["test"]}`)
	req, _ := http.NewRequest("POST", "https://tmt.tencentcloudapi.com/", nil)
	signTencent(req, payload, TranslationEndpoint{Endpoint: Endpoint{token: "test-secret"}, secretID: "test-id", Region: "ap-guangzhou"}, time.Unix(1700000000, 0))
	// 使用 Python hashlib/hmac 独立计算的固定合成测试向量。
	want := "TC3-HMAC-SHA256 Credential=test-id/2023-11-14/tmt/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=5aca4870fe3cdb57b1e2cce5c4c5fd6ab4db58d8ceda91fe97398713742eb799"
	if req.Header.Get("Authorization") != want {
		t.Fatal("TC3 signature mismatch")
	}
	if req.Header.Get("X-TC-Timestamp") != "1700000000" || req.Header.Get("X-TC-Version") != "2018-03-21" || req.Header.Get("X-TC-Region") != "ap-guangzhou" {
		t.Fatal("missing Tencent headers")
	}
}

func TestTencentTranslation(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
	}{
		{"success", `{"Response":{"TargetTextList":["test"],"RequestId":"synthetic"}}`, 200},
		{"provider error", `{"Response":{"Error":{"Code":"AuthFailure","Message":"secret-details"},"TargetTextList":["test"]}}`, 502},
		{"missing", `{"Response":{}}`, 502},
		{"multiple", `{"Response":{"TargetTextList":["a","b"]}}`, 502},
		{"blank", `{"Response":{"TargetTextList":[" "]}}`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" || r.Method != "POST" || r.Header.Get("X-TC-Action") != "TextTranslateBatch" || !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=test-id/") {
					t.Error("invalid Tencent request")
				}
				var body struct {
					Source, Target string
					ProjectId      int
					SourceTextList []string
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Source != "auto" || body.Target != "en" || body.ProjectId != 0 || len(body.SourceTextList) != 1 || body.SourceTextList[0] != "测试" {
					t.Error("translation payload mismatch")
				}
				_, _ = io.WriteString(w, tc.response)
			})
			s.config.Translation.Provider = "tencent"
			s.config.Translation.secretID = "test-id"
			s.config.Translation.Region = "ap-guangzhou"
			w := call(s, "POST", "/v1/translate", `{"text":"测试","source_lang":"AUTO","target_lang":"EN"}`)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "secret-details") {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == 200 && !strings.Contains(w.Body.String(), `"data":"test"`) {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestTencentConfig(t *testing.T) {
	t.Setenv("TEST_CLIENT_TOKEN", testToken)
	t.Setenv("TEST_TENCENT_ID", "test-id")
	t.Setenv("TEST_TENCENT_KEY", "test-secret")
	base := func() Config {
		return Config{Clients: []Client{{ID: "test", TokenEnv: "TEST_CLIENT_TOKEN", RequestsPerMinute: 10}}, Translation: TranslationEndpoint{Provider: "tencent", SecretIDEnv: "TEST_TENCENT_ID", Endpoint: Endpoint{TokenEnv: "TEST_TENCENT_KEY"}}}
	}
	c := base()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Translation.URL != "https://tmt.tencentcloudapi.com/" || c.Translation.Region != "ap-guangzhou" || c.Translation.secretID != "test-id" || c.Translation.token != "test-secret" {
		t.Fatal("Tencent defaults or credentials not loaded")
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Translation.Provider = "unknown" },
		func(c *Config) { c.Translation.SecretIDEnv = "MISSING_TEST_SECRET_ID" },
		func(c *Config) { c.Translation.TokenEnv = "" },
		func(c *Config) { c.Translation.URL = "https://example.com/path" },
		func(c *Config) { c.Translation.URL = "https://example.com/?" },
		func(c *Config) { c.Translation.URL = "http://example.com/" },
		func(c *Config) { c.Translation.Region = "bad\nregion" },
	} {
		c := base()
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("invalid Tencent config accepted")
		}
	}
}
