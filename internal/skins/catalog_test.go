package skins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testManifest = `schema_version = 1
id = "test-skin"
name = "测试皮肤"
version = "1.0.0"
base = "fluent"
preview = "images/preview.png"
toolbar_stylesheet = "toolbar.css"
[supports]
layouts = ["horizontal"]
themes = ["dark"]
[candidate_window]
min_width_dip = 240
[candidate_window.decoration]
top_inset_dip = 20
width_dip = 200
[candidate.dark]
accent = "#123456"
show_selected_bar = false
`

func TestBuiltinSourceAndFilters(t *testing.T) {
	var source struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(Source, &source); err != nil {
		t.Fatal(err)
	}
	packages, invalid, err := Catalog("", "horizontal", "light")
	if err != nil || invalid != 0 || len(packages) != 4 {
		t.Fatal(packages, invalid, err)
	}
	for _, p := range packages {
		if len(p.Resources) != 4 || !p.Builtin {
			t.Fatal(p)
		}
		for _, r := range p.Resources {
			data, kind, err := ReadResource("", p.ID, r.Path)
			if err != nil || kind != "text/css; charset=utf-8" {
				t.Fatal(r, err)
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != source.SHA256[p.ID+"/"+r.Path] {
				t.Fatal("asset provenance mismatch", p.ID, r.Path)
			}
		}
	}
	if !strings.Contains(string(License()), "GNU GENERAL PUBLIC LICENSE") {
		t.Fatal("missing license")
	}
}
func TestManifestValidation(t *testing.T) {
	p, err := Parse([]byte(testManifest), "test-skin")
	if err != nil || !p.Matches("horizontal", "dark") || p.Matches("vertical", "dark") {
		t.Fatal(p, err)
	}
	for _, pair := range [][2]string{{`base = "fluent"`, `base = "other"`}, {`min_width_dip = 240`, `min_width_dip = -1`}, {`min_width_dip = 240`, `min_width_dip = nan`}, {`width_dip = 200`, `width_dip = 0`}, {`themes = ["dark"]`, `themes = ["dark", "dark"]`}, {`themes = ["dark"]`, `themes = ["anything"]`}, {`preview = "images/preview.png"`, `preview = "../private.png"`}, {`toolbar_stylesheet = "toolbar.css"`, `toolbar_stylesheet = "secrets.txt"`}, {`id = "test-skin"`, `id = "another"`}} {
		if _, err := Parse([]byte(strings.Replace(testManifest, pair[0], pair[1], 1)), "test-skin"); err == nil {
			t.Fatal("invalid manifest accepted", pair)
		}
	}
}
func TestPackageBoundariesAndMissingResources(t *testing.T) {
	root := t.TempDir()
	skin := filepath.Join(root, "test-skin")
	if err := os.MkdirAll(filepath.Join(skin, "images"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"skin.toml": testManifest, "images/preview.png": "synthetic image bytes", "toolbar.css": "body {color:red}", "private.env": "never serve"} {
		if err := os.WriteFile(filepath.Join(skin, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	packages, invalid, err := Catalog(root, "horizontal", "dark")
	if err != nil || invalid != 0 || len(packages) != 5 {
		t.Fatal(packages, invalid, err)
	}
	filtered, _, err := Catalog(root, "vertical", "light")
	if err != nil || len(filtered) != 4 {
		t.Fatal(filtered, err)
	}
	for _, name := range []string{"private.env", "../private.png", "images/../../private.png", "/etc/passwd", "skin.toml/../private.env", "images\\preview.png"} {
		if _, _, err := ReadResource(root, "test-skin", name); err == nil {
			t.Fatal("unsafe resource accepted", name)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.png")
	os.WriteFile(outside, []byte("private"), 0600)
	if err := os.Symlink(outside, filepath.Join(skin, "escape.png")); err == nil {
		if _, _, err = ReadResource(root, "test-skin", "escape.png"); err == nil {
			t.Fatal("symlink escaped package")
		}
		os.Remove(filepath.Join(skin, "escape.png"))
	}
	os.Remove(filepath.Join(skin, "toolbar.css"))
	if _, err := Load(root, "test-skin"); err == nil {
		t.Fatal("missing declared resource accepted")
	}
}
