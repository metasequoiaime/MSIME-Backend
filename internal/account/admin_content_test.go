package account

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminContentDetails(t *testing.T) {
	db := testStore(t)
	ctx := context.Background()
	a := &Service{store: db}
	user := complete(t, db, Identity{"email", "private-author@example.test"})
	for _, query := range []string{
		`INSERT INTO community_skins(id,owner_id,name,description,design) VALUES('skin-detail',$1,'Skin','Description','{"color":"#ffffff"}')`,
		`INSERT INTO community_resources(id,owner_id,kind,name,content) VALUES('dict-detail',$1,'dictionary','Words','{"entries":[{"kind":"pinyin","code":"ni","word":"你","weight":10}]}'),('reply-detail',$1,'reply','Reply','{"prompt":"<script>alert(1)</script>"}')`,
		`INSERT INTO community_skin_downloads(skin_id,user_id) VALUES('skin-detail',$1)`,
		`INSERT INTO community_skin_ratings(skin_id,user_id,stars) VALUES('skin-detail',$1,4)`,
		`INSERT INTO community_resource_saves(resource_id,user_id) VALUES('dict-detail',$1)`,
		`INSERT INTO community_resource_ratings(resource_id,user_id,stars) VALUES('dict-detail',$1,5)`,
	} {
		if _, err := db.pool.Exec(ctx, query, user.User.ID); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.AdminHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		path, content string
		uses, rating  int
	}{
		{"skins/skin-detail", "#ffffff", 1, 4}, {"dictionaries/dict-detail", "你", 1, 5}, {"replies/reply-detail", "<script>alert(1)</script>", 0, 0},
	} {
		w := call("GET", "/api/"+tc.path, "")
		var data struct {
			Owner            string `json:"owner_id"`
			Content          json.RawMessage
			Downloads, Saves int
			Rating           float64 `json:"rating_average"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &data) != nil || data.Owner != user.User.ID || data.Downloads+data.Saves != tc.uses || data.Rating != float64(tc.rating) {
			t.Fatal(w.Code, w.Body.String())
		}
		// Decode the JSON string escaping before checking the returned content.
		var content any
		if err := json.Unmarshal(data.Content, &content); err != nil {
			t.Fatal(err)
		}
		normalized, _ := json.Marshal(content)
		expected, _ := json.Marshal(tc.content)
		if !strings.Contains(string(normalized), string(expected)) {
			t.Fatal(string(normalized))
		}
		if strings.Contains(w.Body.String(), "private-author@example.test") {
			t.Fatal("identity leaked")
		}
	}
	for _, path := range []string{"skins/missing", "dictionaries/reply-detail", "replies/dict-detail"} {
		if w := call("GET", "/api/"+path, ""); w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	if w := call("GET", "/api/skins/bad/path", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/skins/skin-detail", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/actions", `{"action":"delete_skin","id":"skin-detail"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("GET", "/api/skins/skin-detail", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	var remaining int
	if err := db.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM community_skin_downloads WHERE skin_id='skin-detail')+(SELECT count(*) FROM community_skin_ratings WHERE skin_id='skin-detail')`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal(remaining, err)
	}
}
