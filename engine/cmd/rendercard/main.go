// Command rendercard looks up a card by name and renders it with a template read
// from a folder or a bundle, writing a PNG. It exists to prove the pipeline end to end, not as the intended
// long-term interface: a future UI calls the same template entry point.
package main

import (
	"context"
	"flag"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

func main() {
	name := flag.String("name", "", "card name to look up (fuzzy match)")
	out := flag.String("o", "card.png", "output PNG path")
	assetsDir := flag.String("assets", "", "directory holding a template, with its manifest.json; empty generates a placeholder template")
	bundle := flag.String("bundle", "", "path to a .mimic bundle holding the template; takes precedence over -assets")
	noArt := flag.Bool("no-art", false, "skip fetching and placing card art")
	noSymbol := flag.Bool("no-symbol", false, "skip fetching and drawing the set symbol")
	fontDir := flag.String("fonts", "", "directory of font overrides, one font per role subfolder (title, body, body-italic, mana, type, info); empty auto-uses ./local-fonts if present, else the embedded defaults")
	face := flag.Int("face", 0, "which face to render, 0 for the front and 1 for a double-faced card's back")
	dpi := flag.Int("dpi", 0, "resolution to render at, clamped to the template's own; 0 renders at the template's authored resolution")
	timeout := flag.Duration("timeout", 30*time.Second, "overall timeout for network work")
	flag.Parse()

	if *name == "" {
		log.Fatal("rendercard: -name is required")
	}
	if err := run(*name, *out, *assetsDir, *bundle, *fontDir, *noArt, *noSymbol, *face, *dpi, *timeout); err != nil {
		log.Fatalf("rendercard: %v", err)
	}
}

func run(name, out, assetsDir, bundle, fontDir string, noArt, noSymbol bool, face, dpi int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	assets, cleanup, err := resolveAssets(assetsDir, bundle)
	if err != nil {
		return err
	}
	defer cleanup()
	tmpl, err := loadTemplate(assets)
	if err != nil {
		return err
	}

	client := card.NewClient(card.WithSymbolCache(card.NewDirCache(symbolCacheDir())))
	data, err := client.FetchByName(ctx, name)
	if err != nil {
		return err
	}
	log.Printf("resolved %q (%s, %s)", data.Name, data.TypeLine, data.SetCode)

	var art image.Image
	if !noArt {
		// A back face has its own art, and Face returns the card itself for 0
		art, err = client.FetchArt(ctx, data.Face(face))
		if err != nil {
			// Art is optional. A frame with text still proves the pipeline
			log.Printf("continuing without art: %v", err)
		}
	}

	var symbol *card.SetSymbol
	if !noSymbol {
		// Like the art, the symbol is optional, and a frame without one still renders
		if symbol, err = client.FetchSetSymbol(ctx, data); err != nil {
			log.Printf("continuing without set symbol: %v", err)
		}
	}

	if fontDir = resolveFontDir(fontDir); fontDir != "" {
		log.Printf("font overrides: %s", fontDir)
	}

	buf, err := render.Render(ctx, tmpl, template.RenderRequest{
		Card:      data,
		Face:      face,
		Art:       art,
		SetSymbol: symbol,
		FontDir:   fontDir,
		DPI:       dpi,
	})
	if err != nil {
		return err
	}
	return writePNG(out, buf.ToImage(8))
}

// symbolCacheDir is where downloaded set symbols are kept between runs, or "" when
// the system has no cache directory, in which case they are downloaded each run
func symbolCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "mimic", "symbols")
}

// defaultFontDirs lists where rendercard looks for font overrides when -fonts
// is not given. local-fonts sits at the repo root, so both a repo-root run and
// an engine-subdir run find it
var defaultFontDirs = []string{"local-fonts", filepath.Join("..", "local-fonts")}

// resolveFontDir returns dir when set, else the first default font directory
// that exists, else "" for the embedded defaults
func resolveFontDir(dir string) string {
	if dir != "" {
		return dir
	}
	for _, cand := range defaultFontDirs {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}

// loadTemplate reads the template an asset provider holds and reports any warnings
func loadTemplate(p template.AssetProvider) (*template.Template, error) {
	t, ds, err := template.Load(p)
	for _, d := range ds {
		log.Print(d)
	}
	if err != nil {
		return nil, err
	}
	log.Printf("template %s serves %d shapes", t.ID(), len(t.Shapes()))
	return t, nil
}

// resolveAssets returns the asset provider to render from. A bundle path takes
// precedence and renders through a ZipAssetProvider; else a loose directory
// renders through an FSAssetProvider; else a placeholder template is generated
// into a temporary directory the caller cleans up
func resolveAssets(dir, bundle string) (template.AssetProvider, func(), error) {
	if bundle != "" {
		p, err := template.NewZipAssetProvider(bundle)
		if err != nil {
			return nil, func() {}, err
		}
		return p, func() { p.Close() }, nil
	}
	if dir != "" {
		return template.NewFSAssetProvider(dir), func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "mimic-placeholder-")
	if err != nil {
		return nil, func() {}, err
	}
	if err := render.WritePlaceholderAssets(tmp, "mimic.placeholder"); err != nil {
		os.RemoveAll(tmp)
		return nil, func() {}, err
	}
	return template.NewFSAssetProvider(tmp), func() { os.RemoveAll(tmp) }, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	log.Printf("wrote %s", path)
	return nil
}
