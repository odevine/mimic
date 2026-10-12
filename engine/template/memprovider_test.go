package template

import (
	"io"
	"testing"
)

func TestMemoryAssets(t *testing.T) {
	m := NewMemoryAssets([]byte(`{}`), map[string][]byte{"a/b.png": []byte("png")})
	if raw, err := m.ReadManifest(); err != nil || string(raw) != "{}" {
		t.Errorf("manifest = %q, %v", raw, err)
	}
	r, err := m.Open("a/b.png")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); string(b) != "png" {
		t.Errorf("file = %q", b)
	}
	for _, rel := range []string{"missing.png", "../a/b.png", "/a/b.png", "a/../a/b.png"} {
		if r, err := m.Open(rel); err == nil {
			r.Close()
			t.Errorf("Open(%q) succeeded", rel)
		}
	}
}
