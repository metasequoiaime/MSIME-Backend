package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
)

// 通过 HTTP、TLS 上游和供应商响应转换验证 Engine 定义的协议示例。
func TestEngineBackendContractOverHTTP(t *testing.T) {
	var spec struct {
		Operations map[string]struct {
			Method        string            `json:"method"`
			Path          string            `json:"path"`
			Authenticated bool              `json:"authenticated"`
			ContentType   string            `json:"content_type"`
			Request       json.RawMessage   `json:"request"`
			Response      json.RawMessage   `json:"response"`
			Query         map[string]string `json:"query"`
		} `json:"operations"`
	}
	data, err := os.ReadFile("../../contracts/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("wrong provider credential")
		}
		key := r.URL.Path[1:]
		if key == "cloud" {
			_, _ = io.WriteString(w, `["SUCCESS",[["ni'hao",["你好"]]]]`)
			return
		}
		if operation, ok := spec.Operations[key]; ok {
			_, _ = w.Write(operation.Response)
		} else {
			http.NotFound(w, r)
		}
	})
	s.config.Cloud.URL += "/cloud"
	s.config.Chat.URL += "/chat"
	s.config.Translation.URL += "/translation"
	s.config.Transcription.URL += "/transcription"
	live := httptest.NewServer(s)
	defer live.Close()
	for key, operation := range spec.Operations {
		t.Run(key, func(t *testing.T) {
			var body io.Reader = bytes.NewReader(operation.Request)
			contentType := operation.ContentType
			if key == "transcription" {
				var b bytes.Buffer
				m := multipart.NewWriter(&b)
				f, _ := m.CreateFormFile("file", "test.wav")
				_, _ = f.Write(testWAV())
				_ = m.WriteField("language", "zh")
				_ = m.WriteField("model", "msime")
				_ = m.Close()
				contentType = m.FormDataContentType()
				body = &b
			}
			query := url.Values{}
			for k, v := range operation.Query {
				query.Set(k, v)
			}
			req, err := http.NewRequest(operation.Method, live.URL+operation.Path+"?"+query.Encode(), body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", contentType)
			if operation.Authenticated {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}
			response, err := live.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var actual, expected any
			if err = json.NewDecoder(response.Body).Decode(&actual); err != nil {
				t.Fatal(err)
			}
			_ = json.Unmarshal(operation.Response, &expected)
			if response.StatusCode != 200 || !reflect.DeepEqual(actual, expected) {
				t.Fatalf("contract mismatch: status=%d actual=%v", response.StatusCode, actual)
			}
			if response.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("response can be cached")
			}
		})
	}
}
