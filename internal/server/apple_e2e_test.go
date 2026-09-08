package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestNativeAppleNetwork(t *testing.T) {
	executable := os.Getenv("MSIME_NATIVE_APPLE")
	if executable == "" {
		t.Skip("run scripts/apple_e2e.py on macOS")
	}
	if runtime.GOOS != "darwin" {
		t.Fatal("Apple client requires macOS")
	}
	var cancelled atomic.Bool
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		text := r.URL.Query().Get("text")
		if text == "cancel" {
			<-r.Context().Done()
			cancelled.Store(true)
			return
		}
		if r.URL.Query().Get("itc") == "ja-t-i0-und" {
			if text != "nihon" {
				t.Error("Japanese query changed")
			}
			_, _ = io.WriteString(w, `["SUCCESS",[["nihon",["日本"]]]]`)
		} else {
			if text != "ni'hao" {
				t.Error("pinyin query changed")
			}
			_, _ = io.WriteString(w, `["SUCCESS",[["ni'hao",["你好"]]]]`)
		}
	})
	live := httptest.NewTLSServer(s)
	defer live.Close()
	certPath := filepath.Join(t.TempDir(), "test.der")
	if err := os.WriteFile(certPath, live.TLS.Certificates[0].Certificate[0], 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable, live.URL, testToken, certPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Apple network client: %v %s", err, output)
	}
	if !cancelled.Load() {
		t.Fatal("Apple cancellation did not reach upstream")
	}
	t.Log(string(output))
}
