package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAITranslation(t *testing.T) {
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		var v chatRequest
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("Authorization") != "Bearer provider-secret" || v.Model != "server-model" || v.Stream || len(v.Messages) != 2 {
			t.Fatalf("invalid upstream request")
		}
		if !strings.Contains(v.Messages[0].Content, "automatically detected") || !strings.Contains(v.Messages[0].Content, "to en") || v.Messages[1].Content != "测试" {
			t.Fatalf("translation instructions or input missing")
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":" Test "},"finish_reason":"stop"}]}`)
	})
	s.config.Translation.Provider = "openai"
	w := call(s, "POST", "/v1/translate", `{"text":"测试","source_lang":"AUTO","target_lang":"en"}`)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"code":200,"data":"Test"}` {
		t.Fatal(w.Code, w.Body.String())
	}
	s.config.Translation.Model = ""
	if err := s.config.Validate(); err == nil {
		t.Fatal("missing translation model accepted")
	}
}

func TestOpenAITranslationRejectsInvalidOutput(t *testing.T) {
	for _, response := range []string{
		`{"choices":[]}`,
		`{"error":"provider-secret"}`,
		`{"choices":[{"message":{"content":" "},"finish_reason":"stop"}]}`,
		`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`,
		`{"choices":[{"message":{"content":"filtered"},"finish_reason":"content_filter"}]}`,
		`not json`,
	} {
		t.Run(response, func(t *testing.T) {
			s := fixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, response) })
			s.config.Translation.Provider = "openai"
			w := call(s, "POST", "/v1/translate", `{"text":"test","source_lang":"en","target_lang":"zh"}`)
			if w.Code != 502 || strings.Contains(w.Body.String(), "provider-secret") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
