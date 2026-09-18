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
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/engine/template/normal"
)

func main() {
	name := flag.String("name", "", "card name to look up (fuzzy match)")
	out := flag.String("o", "card.png", "output PNG path")
	assetsDir := flag.String("assets", "", "template asset directory; empty generates placeholder assets")
	tmplName := flag.String("template", "normal", "template name")
	noArt := flag.Bool("no-art", false, "skip fetching and placing card art")
	fontDir := flag.String("fonts", "", "directory of font overrides (e.g. Beleren.ttf, Plantin.ttf); empty uses the embedded defaults")
	timeout := flag.Duration("timeout", 30*time.Second, "overall timeout for network work")
	flag.Parse()

	if *name == "" {
		log.Fatal("rendercard: -name is required")
	}
	if err := run(*name, *out, *assetsDir, *tmplName, *fontDir, *noArt, *timeout); err != nil {
		log.Fatalf("rendercard: %v", err)
	}
}

func run(name, out, assetsDir, tmplName, fontDir string, noArt bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dir, cleanup, err := resolveAssets(assetsDir)
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
	if fontDir != "" {
		if nt, ok := tmpl.(*normal.Template); ok {
			nt.FontDir = fontDir
		}
	}

	buf, err := tmpl.Render(ctx, template.RenderRequest{
		Card:   data,
		Art:    art,
		Assets: template.NewFSAssetProvider(dir),
	})
	if err != nil {
		return err
	}
	return writePNG(out, buf.ToImage(8))
}

// resolveAssets returns the asset directory to render from. An empty dir means
// generate placeholder assets into a temporary directory the caller cleans up
func resolveAssets(dir string) (string, func(), error) {
	if dir != "" {
		return dir, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "mimic-placeholder-")
	if err != nil {
		return "", func() {}, err
	}
	if err := normal.WritePlaceholderAssets(tmp); err != nil {
		os.RemoveAll(tmp)
		return "", func() {}, err
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
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
