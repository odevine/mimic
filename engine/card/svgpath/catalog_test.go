package svgpath

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/odevine/impasto/raster"
)

// TestCatalog parses and draws every icon in the mtg-vectors catalog. It reads
// the optimized release zip named by MTG_VECTORS_ZIP and is skipped without it,
// since the catalog is not part of this repository. It is the regression suite
// for the renderer: what the catalog uses is what the renderer has to handle
func TestCatalog(t *testing.T) {
	zipPath := os.Getenv("MTG_VECTORS_ZIP")
	if zipPath == "" {
		t.Skip("set MTG_VECTORS_ZIP to the mtg-vectors optimized release zip to run")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	var parsed, unsupported int
	reasons := map[string][]string{}
	for _, f := range zr.File {
		// The rarity files of each set, the ones a card is drawn with
		parts := strings.Split(f.Name, "/")
		if len(parts) != 3 || parts[0] != "set" || !strings.HasPrefix(parts[1], "") || parts[1][0] == '.' {
			continue
		}
		switch strings.TrimSuffix(parts[2], ".svg") {
		case "C", "U", "R", "M", "T":
		default:
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		icon, err := Parse(data)
		if err != nil {
			if errors.Is(err, ErrUnsupported) {
				unsupported++
				reasons[err.Error()] = append(reasons[err.Error()], f.Name)
				continue
			}
			t.Errorf("%s: %v", f.Name, err)
			continue
		}
		parsed++

		// Draw at a modest size, which must finish and put ink down
		ink := icon.Bounds()
		if ink.W <= 0 || ink.H <= 0 {
			t.Errorf("%s: no ink bounds", f.Name)
			continue
		}
		const h = 96
		k := float64(h) / float64(ink.H)
		w := int(float64(ink.W)*k) + 1
		buf := raster.MustNewBuffer(w+4, h+4)
		icon.Draw(buf, Translate(2, 2).Mul(ScaleBy(k, k)).Mul(Translate(-float64(ink.X), -float64(ink.Y))))
		var covered int
		for i := 3; i < len(buf.Pix); i += 4 {
			if buf.Pix[i] > 0.5 {
				covered++
			}
		}
		if covered == 0 {
			t.Errorf("%s: drew nothing", f.Name)
		}
	}
	t.Logf("parsed %d icons, %d unsupported", parsed, unsupported)
	var keys []string
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%d x %s, e.g. %v", len(reasons[k]), k, reasons[k][:min(3, len(reasons[k]))])
	}
}
