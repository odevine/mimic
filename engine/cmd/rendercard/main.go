// Command rendercard looks up a card by name and renders it with a template read
// from a folder or a bundle, writing a PNG. It exists to prove the pipeline end to end, not as the intended
// long-term interface: a future UI calls the same template entry point.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
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
	fontDir := flag.String("fonts", "", "directory of font files to choose overrides from, subfolders included; empty auto-uses ./local-fonts if present")
	var overrides fontFlags
	flag.Var(&overrides, "font", "draw a role with a font from the -fonts directory, as role=\"Family Style\" such as title=\"Gotham Medium\"; repeat for more roles")
	face := flag.Int("face", 0, "which face to render, 0 for the front and 1 for a double-faced card's back")
	dpi := flag.Int("dpi", 0, "resolution to render at, clamped to the template's own; 0 renders at the template's authored resolution")
	timeout := flag.Duration("timeout", 30*time.Second, "overall timeout for network work")
	flag.Parse()

	if *name == "" {
		log.Fatal("rendercard: -name is required")
	}
	if err := run(*name, *out, *assetsDir, *bundle, *fontDir, overrides, *noArt, *noSymbol, *face, *dpi, *timeout); err != nil {
		log.Fatalf("rendercard: %v", err)
	}
}

func run(name, out, assetsDir, bundle, fontDir string, overrides fontFlags, noArt, noSymbol bool, face, dpi int, timeout time.Duration) error {
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

	pool := loadFontPool(resolveFontDir(fontDir), overrides, tmpl.Fonts())

	buf, err := render.Render(ctx, tmpl, template.RenderRequest{
		Card:      data,
		Face:      face,
		Art:       art,
		SetSymbol: symbol,
		DPI:       dpi,

		FontPool:      pool,
		FontOverrides: overrides,
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

// fontFlags collects the repeated -font flag as a role to font name map
type fontFlags map[string]string

func (f *fontFlags) String() string {
	var parts []string
	for role, name := range *f {
		parts = append(parts, role+"="+name)
	}
	return strings.Join(parts, ",")
}

// Set reads one role=name pair, splitting at the first equals sign, since a font
// name has none
func (f *fontFlags) Set(v string) error {
	role, name, ok := strings.Cut(v, "=")
	role, name = strings.TrimSpace(role), strings.TrimSpace(name)
	if !ok || role == "" || name == "" {
		return fmt.Errorf("%q is not role=\"Family Style\"", v)
	}
	if *f == nil {
		*f = fontFlags{}
	}
	(*f)[role] = name
	return nil
}

// loadFontPool indexes the font directory and says what it found, and warns about
// each override that names a role the template lacks or a font the pool lacks
func loadFontPool(dir string, overrides fontFlags, cat *fonts.Catalog) *fonts.Pool {
	pool := fonts.NewPool(dir)
	if dir != "" {
		log.Printf("font pool: %s (%d fonts)", dir, len(pool.Fonts()))
	}
	for _, r := range pool.Rejected() {
		log.Printf("font pool: skipping %s: %v", r.Path, r.Err)
	}
	for _, p := range pool.Duplicates() {
		log.Printf("font pool: skipping %s, another file has the same name", p)
	}
	for role, name := range overrides {
		switch _, found := pool.Lookup(name); {
		case !cat.Knows(fonts.Role(role)):
			log.Printf("font override: the template has no role %q", role)
		case !found:
			log.Printf("font override: %q is not in the pool, so %s keeps its own font", name, role)
		}
	}
	return pool
}

// defaultFontDirs lists where rendercard looks for font overrides when -fonts
// is not given. local-fonts sits at the repo root, so both a repo-root run and
// an engine-subdir run find it
var defaultFontDirs = []string{"local-fonts", filepath.Join("..", "local-fonts")}

// resolveFontDir returns dir when set, else the first default font directory
// that exists, else "" for no pool
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
// renders through an FSAssetProvider; else the built-in placeholder template
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
	return render.PlaceholderAssets(), func() {}, nil
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
