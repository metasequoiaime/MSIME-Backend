package account

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const communityFixture = `{"background":15266027,"keyBackground":16777215,"keyForeground":1516829,"accent":1596487,"actionBackground":1596487,"cornerRadius":8,"borderWidth":0,"shadow":0,"pattern":0,"monospaced":false}`

func TestCommunityDesignValidation(t *testing.T) {
	if _, e := parseCommunityDesign(json.RawMessage(communityFixture)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{}`, `null`, strings.Replace(communityFixture, `"cornerRadius":8`, `"cornerRadius":21`, 1), strings.Replace(communityFixture, `"pattern":0`, `"pattern":4`, 1), strings.Replace(communityFixture, `"monospaced":false`, `"monospaced":false,"photo":"aW52YWxpZA=="`, 1)} {
		if _, e := parseCommunityDesign(json.RawMessage(raw)); e == nil {
			t.Fatal("accepted", raw)
		}
	}
}
func TestCommunityPublishDownloadRatingOwnershipAndRestart(t *testing.T) {
	store := testStore(t)
	owner := complete(t, store, Identity{"apple", "owner"})
	user := complete(t, store, Identity{"apple", "reader"})
	a := &Service{store: store}
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		Mount(mux, a)
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	id := "ab334455-1234-1234-1234-123456789abc"
	path := "/v1/community/skins/" + id
	body := `{"id":"` + id + `","name":"测试皮肤","description":"示例","design":` + communityFixture + `}`
	if w := request("POST", "/v1/community/skins", body, ""); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/v1/community/skins", body, owner.AccessToken); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/v1/community/skins", body, owner.AccessToken); w.Code != 200 {
		t.Fatal("retry", w.Code, w.Body.String())
	}
	if w := request("POST", "/v1/community/skins", strings.Replace(body, "测试皮肤", "变更皮肤", 1), owner.AccessToken); w.Code != 409 {
		t.Fatal("changed retry", w.Code)
	}
	if w := request("POST", "/v1/community/skins", body, user.AccessToken); w.Code != 409 {
		t.Fatal("identity", w.Code)
	}
	if w := request("PUT", path+"/rating", `{"stars":5}`, user.AccessToken); w.Code != 403 {
		t.Fatal("rating before download", w.Code)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request("POST", path+"/download", `{}`, user.AccessToken)
			if w.Code != 200 {
				t.Error(w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	for _, stars := range []string{"5", "3"} {
		if w := request("PUT", path+"/rating", `{"stars":`+stars+`}`, user.AccessToken); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request("PUT", path+"/rating", `{"stars":6}`, user.AccessToken); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", path+"/rating", `{"stars":5}`, owner.AccessToken); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("DELETE", path, ``, user.AccessToken); w.Code != 404 {
		t.Fatal("owner check", w.Code)
	}
	a = &Service{store: store}
	w := request("GET", path, ``, user.AccessToken)
	var v CommunitySkin
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Downloads != 1 || v.RatingCount != 1 || v.RatingAverage != 3 || v.MyRating != 3 || v.Owned {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("GET", "/v1/community/skins", ``, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "测试皮肤") || strings.Contains(w.Body.String(), user.User.ID) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("DELETE", path, ``, owner.AccessToken); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request("GET", path, ``, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
