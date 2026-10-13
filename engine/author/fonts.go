package author

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/manifest"
)

// packFonts reads the font files a template ships and returns each font as the
// manifest holds it, with its hash and the names it carries filled in, beside the
// bytes of each file by its path in the bundle. A font that cannot be read is
// reported at its path and left out
func packFonts(fsys fs.FS, defs map[string]manifest.FontFile) (map[string]manifest.FontFile, map[string][]byte, manifest.Diagnostics) {
	if len(defs) == 0 {
		return nil, nil, nil
	}
	var ds manifest.Diagnostics
	out := make(map[string]manifest.FontFile, len(defs))
	data := make(map[string][]byte, len(defs))
	for _, id := range sortedKeys(defs) {
		f := defs[id]
		at := "fonts." + id
		fail := func(format string, args ...any) {
			ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: at + ".file", Message: fmt.Sprintf(format, args...)})
			out[id] = f
		}
		raw, err := readFont(fsys, f.File)
		if err != nil {
			fail("%s cannot be read: %v", f.File, err)
			continue
		}
		sum := sha256.Sum256(raw)
		got := hex.EncodeToString(sum[:])
		if f.SHA256 != "" && !strings.EqualFold(f.SHA256, got) {
			fail("%s does not match the sha256 written for it, which is %s and the file's is %s", f.File, f.SHA256, got)
			continue
		}
		family, style, err := fonts.Describe(raw)
		if err != nil {
			fail("%s is not a font: %v", f.File, err)
			continue
		}
		f.SHA256 = got
		if f.Family == "" {
			f.Family = family
		}
		if f.Style == "" {
			f.Style = style
		}
		out[id] = f
		data[f.File] = raw
	}
	return out, data, ds
}

func readFont(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, fonts.MaxFontBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > fonts.MaxFontBytes {
		return nil, fmt.Errorf("is over the limit of %d bytes", fonts.MaxFontBytes)
	}
	return raw, nil
}
