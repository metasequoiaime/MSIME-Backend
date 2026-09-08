package server

import (
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 仅在 native_e2e.py 创建的一次性容器中手动启用此测试。
func TestNativeCloudClients(t *testing.T) {
	clients := os.Getenv("MSIME_NATIVE_CLIENTS")
	if clients == "" {
		t.Skip("run scripts/native_e2e.py to exercise native clients")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("native trust setup is restricted to a disposable container")
	}
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("text") != "ni'hao" {
			t.Error("spelling changed in transit")
		}
		_, _ = io.WriteString(w, `["SUCCESS",[["ni'hao",["你好"]]]]`)
	})
	live := httptest.NewTLSServer(s)
	defer live.Close()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: live.TLS.Certificates[0].Certificate[0]})
	if err := os.WriteFile("/usr/local/share/ca-certificates/msime-native-e2e.crt", cert, 0644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("update-ca-certificates").CombinedOutput(); err != nil {
		t.Fatalf("container trust setup failed: %v %s", err, output)
	}
	for _, client := range filepath.SplitList(clients) {
		t.Run(filepath.Base(client), func(t *testing.T) {
			if output, err := exec.Command(client, live.URL+"/v1/cloud/candidates", testToken).CombinedOutput(); err != nil {
				t.Fatalf("native cloud request failed: %v %s", err, output)
			}
		})
	}
}

func TestNativeServices(t *testing.T) {
	client := os.Getenv("MSIME_NATIVE_SERVICES")
	if client == "" {
		t.Skip("run scripts/native_e2e.py")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("disposable container required")
	}
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("provider credential mismatch")
		}
		switch r.URL.Path {
		case "/translate":
			_, _ = io.WriteString(w, `{"code":200,"data":"test"}`)
		case "/transcribe":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("model") != "server-model" {
				t.Error("model was not fixed by service")
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			audio, _ := io.ReadAll(f)
			if !validWAV(audio) {
				t.Error("invalid client WAV")
			}
			_, _ = io.WriteString(w, `{"text":"测试"}`)
		case "/chat":
			var request chatRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Model != "server-model" || request.EnableThinking != nil || request.Thinking != nil {
				t.Error("provider settings not normalized")
			}
			content := "整理结果"
			if request.ResponseFormat != nil {
				content = `{"candidates":[{"text":"你好"}]}`
			}
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": message{Role: "assistant", Content: content}}}})
		default:
			t.Error("unexpected upstream route")
			http.NotFound(w, r)
		}
	})
	s.config.Chat.URL += "/chat"
	s.config.Translation.URL += "/translate"
	s.config.Transcription.URL += "/transcribe"
	live := httptest.NewTLSServer(s)
	defer live.Close()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: live.TLS.Certificates[0].Certificate[0]})
	if err := os.WriteFile("/usr/local/share/ca-certificates/msime-native-e2e.crt", cert, 0644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("update-ca-certificates").CombinedOutput(); err != nil {
		t.Fatalf("trust setup: %v %s", err, output)
	}
	output, err := exec.Command(client, live.URL, testToken).CombinedOutput()
	if err != nil {
		t.Fatalf("native services: %v %s", err, output)
	}
	t.Log(string(output))
}
