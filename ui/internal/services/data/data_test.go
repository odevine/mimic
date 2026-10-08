package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/fontdir"
	"github.com/odevine/mimic/ui/internal/workspace"
)

func roleLine(st FontsStatus, role string) FontRole {
	for _, r := range st.Roles {
		if r.Role == role {
			return r
		}
	}
	return FontRole{}
}

func fontsService(dir fontdir.Dir) *Service {
	return New(&workspace.Workspace{Fonts: dir}, func(string) error { return nil })
}

func TestFontsAddAndRemove(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "engine", "fonts", "embedded", "body", "Merriweather-Italic.ttf"))
	if err != nil {
		t.Skipf("engine fonts not in this checkout: %v", err)
	}
	svc := fontsService(fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindManaged})

	st := svc.Fonts()
	if !st.Writable || len(st.Roles) != 6 {
		t.Fatalf("status = %+v, want a writable folder with six roles", st)
	}
	if got := roleLine(st, "info"); got.Source != "borrowed" || got.Borrows != "body" {
		t.Errorf("info = %+v, want borrowed from body", got)
	}

	src := filepath.Join(t.TempDir(), "Mine.ttf")
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = svc.AddFont("title", src)
	if err != nil {
		t.Fatal(err)
	}
	if got := roleLine(st, "title"); got.Source != "user" || got.File != "Mine.ttf" || got.Style != "Italic" {
		t.Errorf("title after upload = %+v, want Mine.ttf from the user", got)
	}

	bad := filepath.Join(t.TempDir(), "Bad.ttf")
	os.WriteFile(bad, []byte("nope"), 0o644)
	if _, err := svc.AddFont("title", bad); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("bad upload: %v, want a bad request", err)
	}

	st, err = svc.RemoveFont("title")
	if err != nil {
		t.Fatal(err)
	}
	if got := roleLine(st, "title"); got.Source != "default" {
		t.Errorf("title after remove = %+v, want the default", got)
	}
}

func TestFontsReadOnlyRefusesWrites(t *testing.T) {
	svc := fontsService(fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindCheckout})
	src := filepath.Join(t.TempDir(), "Mine.ttf")
	os.WriteFile(src, []byte("x"), 0o644)
	if _, err := svc.AddFont("title", src); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("upload to a checkout: %v, want a conflict", err)
	}
	if _, err := svc.RemoveFont("title"); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("remove from a checkout: %v, want a conflict", err)
	}
}

func TestSymbolRevalidatesByTag(t *testing.T) {
	svc := fontsService(fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindManaged})
	img, tag, err := svc.Symbol("W", 32, "")
	if err != nil || img == nil || tag == "" {
		t.Fatalf("symbol = %v, %q, %v, want a pip with a tag", img, tag, err)
	}
	if again, _, err := svc.Symbol("W", 32, tag); err != nil || again != nil {
		t.Errorf("revalidation = %v, %v, want no image", again, err)
	}
	if _, _, err := svc.Symbol("NOPE", 32, ""); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("unknown code: %v, want not found", err)
	}
}
