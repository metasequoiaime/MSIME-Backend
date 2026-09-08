package account

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var personalInputCode = regexp.MustCompile(`^[a-zA-Z';]{1,256}$`)
var personalEnglishCode = regexp.MustCompile(`^[a-zA-Z]{1,64}$`)
var personalQuickCode = regexp.MustCompile(`^[a-zA-Z0-9]{1,32}$`)

func (a *Service) dictionaryQuery(w http.ResponseWriter, r *http.Request) {
	p, ok := a.principal(w, r, false)
	if !ok {
		return
	}
	var v struct {
		Text    string `json:"text"`
		Kind    string `json:"kind"`
		Scheme  string `json:"scheme"`
		Profile string `json:"profile"`
		Limit   int    `json:"limit"`
	}
	if !read(w, r, &v) {
		return
	}
	if v.Kind == "" {
		v.Kind = "pinyin"
	}
	if v.Scheme == "" {
		v.Scheme = "pinyin"
	}
	if v.Profile == "" {
		v.Profile = "xiaohe"
	}
	if v.Limit == 0 {
		v.Limit = 20
	}
	if v.Limit < 1 || v.Limit > 200 || (v.Profile != "xiaohe" && v.Profile != "ziranma" && v.Profile != "shoudao" && v.Profile != "microsoft") {
		writeError(w, 400, "invalid_dictionary_query")
		return
	}
	operation := "candidates"
	switch v.Kind {
	case "pinyin", "jianpin":
		if !personalInputCode.MatchString(v.Text) || (v.Scheme != "pinyin" && v.Scheme != "shuangpin") {
			writeError(w, 400, "invalid_input_code")
			return
		}
		if v.Kind == "jianpin" {
			operation = "jianpin"
		}
	case "wubi":
		if len(v.Text) > 4 || !personalEnglishCode.MatchString(v.Text) {
			writeError(w, 400, "invalid_wubi_code")
			return
		}
		v.Scheme = "wubi"
	case "english":
		if !personalEnglishCode.MatchString(v.Text) {
			writeError(w, 400, "invalid_english_prefix")
			return
		}
		operation = "english"
		v.Text = strings.ToLower(v.Text)
	case "quick":
		if !personalQuickCode.MatchString(v.Text) {
			writeError(w, 400, "invalid_quick_code")
			return
		}
		operation = "quick"
	default:
		writeError(w, 400, "invalid_dictionary_kind")
		return
	}
	query := map[string]any{"operation": operation, "text": v.Text, "scheme": v.Scheme, "profile": v.Profile, "limit": v.Limit}
	out, err := a.engine.QuerySnapshot(r.Context(), map[string]any{"operation": "personal_query", "query": query}, func(ctx context.Context, writer io.Writer) error {
		return a.store.StreamDictionarySnapshot(ctx, p.UserID, func(raw json.RawMessage) error {
			if _, err := writer.Write(raw); err != nil {
				return err
			}
			_, err := writer.Write([]byte{'\n'})
			return err
		})
	})
	if err != nil {
		a.dictionaryError(w, err)
		return
	}
	write(w, 200, out)
}
