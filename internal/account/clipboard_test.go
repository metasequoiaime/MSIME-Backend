package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestClipboardConsentIsolationLimitAndDeletion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	one := complete(t, s, Identity{"email", "one@example.test"})
	two := complete(t, s, Identity{"email", "two@example.test"})
	mux := http.NewServeMux()
	Mount(mux, &Service{store: s})
	call := func(method, path, token, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	path := "/v1/users/me/clipboard"
	call("GET", path, "device-token", "", 401)
	call("POST", path, one.AccessToken, `{"text":"synthetic"}`, 403)
	call("PUT", path+"/settings", one.AccessToken, `{}`, 400)
	call("PUT", path+"/settings", one.AccessToken, `{"enabled":true}`, 200)
	call("POST", path, one.AccessToken, `{"text":""}`, 400)
	b, _ := json.Marshal(map[string]string{"text": strings.Repeat("😀", 2001)})
	call("POST", path, one.AccessToken, string(b), 400)
	b, _ = json.Marshal(map[string]string{"text": strings.Repeat("测", 4000)})
	call("POST", path, one.AccessToken, string(b), 200)
	first := call("POST", path, one.AccessToken, `{"text":"synthetic"}`, 200)
	var item ClipboardItem
	if e := json.Unmarshal(first.Body.Bytes(), &item); e != nil {
		t.Fatal(e)
	}
	duplicate := call("POST", path, one.AccessToken, `{"text":"synthetic"}`, 200)
	var other ClipboardItem
	json.Unmarshal(duplicate.Body.Bytes(), &other)
	if item.ID != other.ID {
		t.Fatal("去重不能创建新条目")
	}
	call("DELETE", path+"/"+item.ID, two.AccessToken, "", 404)
	w := call("GET", path, two.AccessToken, "", 200)
	if strings.Contains(w.Body.String(), "synthetic") {
		t.Fatal("跨用户泄露")
	}
	var wg sync.WaitGroup
	for i := 0; i < 70; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, e := s.AddClipboard(ctx, one.User.ID, fmt.Sprintf("synthetic-%d", i)); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	items, e := s.ListClipboard(ctx, one.User.ID, "")
	if e != nil || len(items) != 50 {
		t.Fatalf("并发上限: %d %v", len(items), e)
	}
	call("DELETE", path+"/"+items[0].ID, one.AccessToken, "", 204)
	call("PUT", path+"/settings", one.AccessToken, `{"enabled":false}`, 200)
	items, e = s.ListClipboard(ctx, one.User.ID, "")
	if e != nil || len(items) != 0 {
		t.Fatal("关闭同步必须清空云端数据", e)
	}
	call("POST", path, one.AccessToken, `{"text":"synthetic"}`, 403)
	call("PUT", path+"/settings", one.AccessToken, `{"enabled":true}`, 200)
	call("POST", path, one.AccessToken, `{"text":"synthetic"}`, 200)
	if e = s.DeleteUser(ctx, one.User.ID); e != nil {
		t.Fatal(e)
	}
	items, e = s.ListClipboard(ctx, one.User.ID, "")
	if e != nil || len(items) != 0 {
		t.Fatal("账号删除未清理数据", e)
	}
}
