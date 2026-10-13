package fonts

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// embeddedBytes reads one of the built-in font files
func embeddedBytes(t *testing.T, file string) []byte {
	t.Helper()
	raw, err := embedded.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// writeFont copies a built-in font file to dir/rel, making any folders
func writeFont(t *testing.T, dir, rel, builtinFile string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, embeddedBytes(t, builtinFile), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// memOpen opens files from a map, the way a bundle's provider would
func memOpen(files map[string][]byte) func(string) (io.ReadCloser, error) {
	return func(name string) (io.ReadCloser, error) {
		b, ok := files[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}
}

const (
	plantinRegular = "embedded/m15/plantin-body.ttf"
	plantinItalic  = "embedded/m15/plantin-italic.ttf"
	gothamMedium   = "embedded/m15/Gotham-Medium.otf"
)
