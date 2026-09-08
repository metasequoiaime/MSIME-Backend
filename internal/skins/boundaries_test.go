package skins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogResourceLimitsAndInvalidPackages(t *testing.T) {
	makePackage := func(t *testing.T) (string, string) {
		root := t.TempDir()
		dir := filepath.Join(root, "test-skin")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		manifest := strings.ReplaceAll(strings.ReplaceAll(testManifest, "preview = \"images/preview.png\"\n", ""), "toolbar_stylesheet = \"toolbar.css\"\n", "")
		if err := os.WriteFile(filepath.Join(dir, "skin.toml"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
		return root, dir
	}
	for _, tc := range []struct {
		name        string
		count, size int
	}{{"individual file", 1, (4 << 20) + 1}, {"total bytes", 5, 4 << 20}, {"resource count", 513, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			root, dir := makePackage(t)
			for i := 0; i < tc.count; i++ {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("asset-%d.css", i)))
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate(int64(tc.size)); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
			if _, err := Load(root, "test-skin"); err == nil {
				t.Fatal("oversized package accepted")
			}
		})
	}
	root, dir := makePackage(t)
	for name, want := range map[string]string{"a.jpg": "image/jpeg", "a.jpeg": "image/jpeg", "a.webp": "image/webp", "a.svg": "image/svg+xml", "a.woff": "font/woff", "a.woff2": "font/woff2"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		data, mime, err := ReadResource(root, "test-skin", name)
		if err != nil || mime != want || string(data) != "fixture" {
			t.Fatal(name, mime, err)
		}
	}
	for _, id := range []string{"../escape", "missing"} {
		if _, _, err := ReadResource(root, id, "a.jpg"); err == nil {
			t.Fatal(id)
		}
	}
	if _, _, err := ReadResource(root, "test-skin", "missing.jpg"); err == nil {
		t.Fatal("unlisted resource exposed")
	}
	if _, _, err := Files(root, "../escape"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err := Load(filepath.Join(root, "missing"), "test-skin"); err == nil {
		t.Fatal("missing root accepted")
	}
	if _, _, err := Catalog(filepath.Join(root, "missing"), "", ""); err == nil {
		t.Fatal("missing root catalog accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "skin.toml"), []byte("broken="), 0600); err != nil {
		t.Fatal(err)
	}
	_, invalid, err := Catalog(root, "", "")
	if err != nil || invalid != 1 {
		t.Fatal(invalid, err)
	}
	if err := os.Remove(filepath.Join(dir, "skin.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "test-skin"); err == nil {
		t.Fatal("missing manifest accepted")
	}
	for i := 0; i < 257; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("skin-%d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Catalog(root, "", ""); err == nil {
		t.Fatal("unbounded catalog accepted")
	}
	for _, raw := range []string{strings.Repeat("x", 65537), strings.Replace(testManifest, `accent = "#123456"`, `accent = "`+strings.Repeat("a", 81)+`"`, 1), strings.Replace(testManifest, `layouts = ["horizontal"]`, `layouts = []`, 1)} {
		if _, err := Parse([]byte(raw), "test-skin"); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
