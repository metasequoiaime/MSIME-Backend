package account

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func signedSnapshot(lines ...string) []byte {
	body := strings.Join(lines, "\n") + "\n"
	hash := sha256.Sum256([]byte(body))
	footer, _ := json.Marshal(map[string]any{"type": "footer", "records": len(lines), "sha256": hex.EncodeToString(hash[:])})
	return append([]byte(body), append(footer, '\n')...)
}
func TestSnapshotDecoderRejectsCorruptionAndAmbiguity(t *testing.T) {
	header := `{"type":"header","format":"msime-dictionary-snapshot","version":1,"revision":2}`
	entry := `{"type":"entry","data":{"id":"test-id","kind":"pinyin","code":"ni'hao","word":"你好","weight":10,"revision":1,"updated_at":"2026-09-08T00:00:00Z"}}`
	position := `{"type":"position","data":{"context":"ni'hao","code":"ni'hao","word":"你好","position":1}}`
	selection := `{"type":"selection","data":{"context":"ni'hao","code":"ni'hao","word":"你好","count":1}}`
	valid := signedSnapshot(header, entry, position, selection)
	count := 0
	if err := decodeDictionarySnapshot(bytes.NewReader(valid), func(snapshotRecord) error { count++; return nil }); err != nil || count != 4 {
		t.Fatal(count, err)
	}
	cases := map[string][]byte{
		"empty":                nil,
		"missing_footer":       []byte(header + "\n"),
		"tampered":             bytes.Replace(valid, []byte("你好"), []byte("拟好"), 1),
		"duplicate_header":     signedSnapshot(header, header),
		"duplicate_entry":      signedSnapshot(header, entry, entry),
		"duplicate_json_key":   signedSnapshot(strings.Replace(header, `"version":1`, `"version":1,"version":1`, 1)),
		"duplicate_nested_key": signedSnapshot(header, strings.Replace(entry, `"weight":10`, `"weight":0,"weight":10`, 1)),
		"unknown_field":        signedSnapshot(strings.Replace(header, `"version":1`, `"version":1,"user_id":"other"`, 1)),
		"missing_count":        signedSnapshot(header, strings.Replace(selection, `,"count":1`, "", 1)),
		"null_count":           signedSnapshot(header, strings.Replace(selection, `"count":1`, `"count":null`, 1)),
		"unsupported_version":  signedSnapshot(strings.Replace(header, `"version":1`, `"version":2`, 1)),
		"future_entry":         signedSnapshot(header, strings.Replace(entry, `"revision":1`, `"revision":3`, 1)),
		"negative_weight":      signedSnapshot(header, strings.Replace(entry, `"weight":10`, `"weight":-1`, 1)),
		"slot_collision":       signedSnapshot(header, position, strings.Replace(position, "你好", "拟好", 1)),
		"out_of_order":         signedSnapshot(header, position, entry),
		"trailing_data":        append(append([]byte{}, valid...), []byte(header+"\n")...),
		"invalid_utf8":         signedSnapshot(header, strings.Replace(entry, "你好", string([]byte{0xff}), 1)),
		"oversized_line":       signedSnapshot(header, strings.Repeat(" ", 65536)+entry),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if err := decodeDictionarySnapshot(bytes.NewReader(input), func(snapshotRecord) error { return nil }); err == nil {
				t.Fatal("accepted invalid snapshot")
			}
		})
	}
	stop := errors.New("staging failed")
	if err := decodeDictionarySnapshot(bytes.NewReader(valid), func(snapshotRecord) error { return stop }); err != stop {
		t.Fatal("staging error lost", err)
	}
}
