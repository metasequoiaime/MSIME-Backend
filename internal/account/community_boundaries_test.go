package account

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"strings"
	"testing"
)

func TestCommunityDesignOptionalFieldsAndPhoto(t *testing.T) {
	base := func() map[string]any {
		var v map[string]any
		if err := json.Unmarshal([]byte(communityFixture), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for key, value := range map[string]any{"keyShape": "bad", "keyMaterial": "bad", "background": 16777216, "gradientEnd": 16777216, "customBorderColor": 16777216, "keyOpacity": 0.1, "patternOpacity": 0.6, "photoShade": 0.9, "photoPosition": 1.1, "photo": make([]byte, 512001), "monospaced": nil} {
		t.Run(key, func(t *testing.T) {
			v := base()
			v[key] = value
			b, _ := json.Marshal(v)
			if _, err := parseCommunityDesign(b); err == nil {
				t.Fatal("invalid field accepted")
			}
		})
	}
	if _, err := parseCommunityDesign([]byte(communityFixture + " {}")); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	for _, width := range []int{8, 1025} {
		var photo bytes.Buffer
		if err := jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, width, 8)), nil); err != nil {
			t.Fatal(err)
		}
		v := base()
		v["photo"] = photo.Bytes()
		b, _ := json.Marshal(v)
		out, err := parseCommunityDesign(b)
		if width > 1024 {
			if err == nil {
				t.Fatal("oversized photo accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(out.Photo))
		if err != nil || cfg.Width != 8 || cfg.Height != 8 {
			t.Fatal("photo not preserved", err)
		}
		v["photo"] = photo.Bytes()[:len(photo.Bytes())-30]
		b, _ = json.Marshal(v)
		if _, err := parseCommunityDesign(b); err == nil {
			t.Fatal("truncated JPEG accepted")
		}
	}
}

func TestCommunityCatalogPaginationAndLimitsHTTP(t *testing.T) {
	db := testStore(t)
	owner := complete(t, db, Identity{"email", "catalog@example.test"})
	a := &Service{store: db}
	mux := http.NewServeMux()
	Mount(mux, a)
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("ab334455-1234-1234-1234-%012d", i)
		if _, err := db.pool.Exec(t.Context(), `INSERT INTO community_skins(id,owner_id,name,description,design) VALUES($1,$2,$3,'',$4)`, id, owner.User.ID, fmt.Sprintf("skin %02d", i), communityFixture); err != nil {
			t.Fatal(err)
		}
		if _, err := db.pool.Exec(t.Context(), `INSERT INTO community_resources(id,owner_id,kind,name,description,content) VALUES($1,$2,'reply',$3,'','{"prompt":"hello"}')`, id, owner.User.ID, fmt.Sprintf("reply %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/v1/community/skins", "/v1/community/resources?kind=reply"} {
		w := apiRequest(t, mux, "GET", path, "", owner.AccessToken, 200)
		var v struct {
			Skins []CommunitySkin     `json:"skins"`
			Items []CommunityResource `json:"items"`
			More  bool                `json:"has_more"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || !v.More || len(v.Skins)+len(v.Items) != 20 {
			t.Fatal(w.Body.String(), err)
		}
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		w = apiRequest(t, mux, "GET", path+separator+"offset=40", "", owner.AccessToken, 200)
		v.Skins = nil
		v.Items = nil
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || v.More || len(v.Skins)+len(v.Items) != 10 {
			t.Fatal(w.Body.String(), err)
		}
	}
	for _, query := range []string{"offset=-1", "offset=100001", "offset=bad", "q=" + strings.Repeat("x", 129)} {
		apiRequest(t, mux, "GET", "/v1/community/skins?"+query, "", "", 400)
	}
	for _, id := range []string{"ab334455x1234-1234-1234-123456789abc", "zb334455-1234-1234-1234-123456789abc"} {
		apiRequest(t, mux, "POST", "/v1/community/skins", `{"id":"`+id+`","name":"new","design":`+communityFixture+`}`, owner.AccessToken, 400)
	}
	apiRequest(t, mux, "POST", "/v1/community/skins", `{"id":"ab334455-1234-1234-1234-999999999999","name":"new","design":{}}`, owner.AccessToken, 400)
	apiRequest(t, mux, "POST", "/v1/community/skins", `{"id":"ab334455-1234-1234-1234-999999999999","name":"new","design":`+communityFixture+`}`, owner.AccessToken, 409)
	apiRequest(t, mux, "POST", "/v1/community/skins/ab334455-1234-1234-1234-999999999999/download", `{}`, owner.AccessToken, 404)
}
