// Command rendercard looks up a card by name and renders it through a template,
// writing a PNG. It exists to prove the pipeline end to end, not as the intended
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
	_ "github.com/odevine/mimic/engine/template/all" // registers every template
)

func main() {
	name := flag.String("name", "", "card name to look up (fuzzy match)")
	out := flag.String("o", "card.png", "output PNG path")
	assetsDir := flag.String("assets", "", "template asset directory; empty generates placeholder assets")
	bundle := flag.String("bundle", "", "path to a .mimic bundle to render from; takes precedence over -assets")
	tmplName := flag.String("template", "normal", "template name")
	noArt := flag.Bool("no-art", false, "skip fetching and placing card art")
	fontDir := flag.String("fonts", "", "directory of font overrides, one font per role subfolder (title, body, body-italic, mana, type, info); empty auto-uses ./local-fonts if present, else the embedded defaults")
	dpi := flag.Int("dpi", 0, "resolution to render at, clamped to the template's own; 0 renders at the template's authored resolution")
	timeout := flag.Duration("timeout", 30*time.Second, "overall timeout for network work")
	flag.Parse()

	if *name == "" {
		log.Fatal("rendercard: -name is required")
	}
	if err := run(*name, *out, *assetsDir, *bundle, *tmplName, *fontDir, *noArt, *dpi, *timeout); err != nil {
		log.Fatalf("rendercard: %v", err)
	}
}

func run(name, out, assetsDir, bundle, tmplName, fontDir string, noArt bool, dpi int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	assets, cleanup, err := resolveAssets(assetsDir, bundle, tmplName)
	if err != nil {
		return err
	}
	defer cleanup()

	client := card.NewClient()
	data, err := client.FetchByName(ctx, name)
	if err != nil {
		return err
	}
	log.Printf("resolved %q (%s, %s)", data.Name, data.TypeLine, data.SetCode)

	var art image.Image
	if !noArt {
		art, err = client.FetchArt(ctx, data)
		if err != nil {
			// Art is optional. A frame with text still proves the pipeline
			log.Printf("continuing without art: %v", err)
		}
	}

	tmpl, err := template.Get(tmplName)
	if err != nil {
		return err
	}
	if fontDir = resolveFontDir(fontDir); fontDir != "" {
		log.Printf("font overrides: %s", fontDir)
	}

	buf, err := tmpl.Render(ctx, template.RenderRequest{
		Card:    data,
		Art:     art,
		Assets:  assets,
		FontDir: fontDir,
		DPI:     dpi,
	})
	if err != nil {
		return err
	}
	return writePNG(out, buf.ToImage(8))
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

// resolveAssets returns the asset provider to render from. A bundle path takes
// precedence and renders through a ZipAssetProvider; else a loose directory
// renders through an FSAssetProvider; else placeholder assets for tmplName are
// generated into a temporary directory the caller cleans up
func resolveAssets(dir, bundle, tmplName string) (template.AssetProvider, func(), error) {
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
	if err := render.WritePlaceholderAssets(tmp, tmplName); err != nil {
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
