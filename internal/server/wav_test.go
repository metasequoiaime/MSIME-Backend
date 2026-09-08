package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testWAV() []byte {
	b := make([]byte, 48)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 40)
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 4)
	return b
}
func TestWAVEnvelope(t *testing.T) {
	if !validWAV(testWAV()) {
		t.Fatal("valid PCM rejected")
	}
	for _, mutate := range []func([]byte){func(b []byte) { b[4]++ }, func(b []byte) { b[16] = 255 }, func(b []byte) { b[20] = 0 }, func(b []byte) { b[28]++ }, func(b []byte) { b[32] = 0 }, func(b []byte) { b[40] = 3 }, func(b []byte) { copy(b[36:], "JUNK") }} {
		b := testWAV()
		mutate(b)
		if validWAV(b) {
			t.Fatal("invalid WAV accepted")
		}
	}
	for n := 0; n < len(testWAV()); n++ {
		if validWAV(testWAV()[:n]) {
			t.Fatal("truncated WAV accepted")
		}
	}
	// 奇数长度的附加块补齐到偶数边界，不改变音频字节。
	b := testWAV()
	b = append(b, []byte{'J', 'U', 'N', 'K', 1, 0, 0, 0, 'x', 0}...)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	if !validWAV(b) {
		t.Fatal("padded ancillary chunk rejected")
	}
}
func TestTranscriptionRejectsEmptyProviderResult(t *testing.T) {
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) })
	var body bytes.Buffer
	m := multipart.NewWriter(&body)
	f, _ := m.CreateFormFile("file", "test.wav")
	_, _ = f.Write(testWAV())
	_ = m.Close()
	r := httptest.NewRequest("POST", "/v1/audio/transcriptions", &body)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Content-Type", m.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func FuzzWAV(f *testing.F) {
	f.Add(testWAV())
	f.Add([]byte("RIFF0000WAVEtest"))
	f.Fuzz(func(t *testing.T, b []byte) { _ = validWAV(b) })
}
