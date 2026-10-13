package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/fontdir"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/templatetest"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

// engineFont reads one of the engine's built-in fonts from the checkout
func engineFont(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "engine", "fonts", "embedded", "m15", name))
	if err != nil {
		t.Skipf("engine fonts not in this checkout: %v", err)
	}
	return raw
}

// writeFontFile puts one of the engine's fonts in a temporary file named name
func writeFontFile(t *testing.T, name, builtin string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, engineFont(t, builtin), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func roleLine(st FontsStatus, role string) FontRole {
	for _, r := range st.Roles {
		if r.Role == role {
			return r
		}
	}
	return FontRole{}
}

func roleNames(st FontsStatus) []string {
	var out []string
	for _, r := range st.Roles {
		out = append(out, r.Role)
	}
	return out
}

// fontsService is a service over a workspace with one template installed, whose
// fonts folder is dir
func fontsService(t *testing.T, kind fontdir.Kind) (*Service, *workspace.Workspace) {
	t.Helper()
	ws := workspacetest.New(t)
	ws.Fonts = fontdir.Dir{Path: filepath.Join(t.TempDir(), "fonts"), Kind: kind}
	ws.FontPool = fontdir.NewPool(ws.Fonts)
	return New(ws, func(string) error { return nil }), ws
}

func assign(ws *workspace.Workspace, role, name string) {
	s := ws.Prefs.Settings()
	s.FontOverrides = map[string]string{role: name}
	ws.Prefs.SetSettings(s)
}

func TestFontsListsTheStandardRolesOnTheirOwnFonts(t *testing.T) {
	svc, _ := fontsService(t, fontdir.KindManaged)
	st := svc.Fonts()
	if !st.Writable || st.Kind != "managed" {
		t.Fatalf("status = %+v, want a writable managed folder", st)
	}
	want := []string{"title", "body", "body-italic", "mana", "small-caps", "info"}
	if got := roleNames(st); len(got) != len(want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	for i, r := range st.Roles {
		if r.Role != want[i] || r.Label == "" || r.Source != "template" || r.Own == "" || r.Family == "" {
			t.Errorf("roles[%d] = %+v, want %s on its own font", i, r, want[i])
		}
	}
	if got := roleLine(st, "title"); got.Own != "Beleren Proxy Bold" || got.Bundled {
		t.Errorf("title = %+v, want the built-in Beleren", got)
	}
	if st.Pool == nil || st.Rejected == nil || st.Duplicates == nil {
		t.Errorf("status = %+v, want empty lists and not null ones", st)
	}
}

func TestFontsListsTheRolesATemplateDefines(t *testing.T) {
	svc, ws := fontsService(t, fontdir.KindManaged)
	templatetest.Spec{
		ID:    "test.saga",
		Fonts: map[string]manifest.FontFile{"script": {File: "font/Script.ttf"}},
		Roles: map[string]manifest.FontRole{
			"saga-chapter": {Label: "Chapter text", Font: "script"},
			"title":        {Label: "Name", Builtin: "mimic-m15-info"},
		},
		FontFiles: map[string][]byte{"font/Script.ttf": engineFont(t, "plantin-italic.ttf")},
	}.WriteLoose(t, filepath.Join(pipeline.LooseDirBases[0], "saga"))
	t.Cleanup(ws.Pipe.Close)

	st := svc.Fonts()
	names := roleNames(st)
	if len(names) != 7 || names[0] != "title" || names[6] != "saga-chapter" {
		t.Fatalf("roles = %v, want the six standard ones then saga-chapter", names)
	}
	saga := roleLine(st, "saga-chapter")
	if saga.Label != "Chapter text" || !saga.Bundled || saga.Own != "Plantin MT Pro Italic" {
		t.Errorf("saga-chapter = %+v, want the bundled italic", saga)
	}
	// The first installed template to declare a role sets its line, and ids sort
	// test.active before test.saga.
	if got := roleLine(st, "title"); got.Own != "Beleren Proxy Bold" {
		t.Errorf("title = %+v, want the first template's font", got)
	}
}

func TestFontsHasNoRolesUntilATemplateIsInstalled(t *testing.T) {
	workspacetest.WithEmptyAssetChain(t)
	fd := fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindManaged}
	ws := &workspace.Workspace{Prefs: workspace.LoadPrefs(), Fonts: fd, FontPool: fontdir.NewPool(fd)}
	st := New(ws, func(string) error { return nil }).Fonts()
	if st.Roles == nil || len(st.Roles) != 0 {
		t.Errorf("roles = %#v, want an empty list", st.Roles)
	}
}

func TestAddedFontsCanBeAssignedAndRemoved(t *testing.T) {
	svc, ws := fontsService(t, fontdir.KindManaged)

	st, err := svc.AddFont(writeFontFile(t, "Mine.otf", "Gotham-Medium.otf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pool) != 1 || st.Pool[0].Name != "Gotham Medium" || st.Pool[0].File != "Mine.otf" || !st.Pool[0].Removable {
		t.Fatalf("pool = %+v, want the added font by its name", st.Pool)
	}
	if got := roleLine(st, "title"); got.Source != "template" {
		t.Errorf("title = %+v, want it unchanged until a font is assigned", got)
	}

	assign(ws, "title", "gotham medium")
	got := roleLine(svc.Fonts(), "title")
	if got.Source != "override" || got.File != "Mine.otf" || got.Family != "Gotham" || got.Override != "gotham medium" {
		t.Errorf("title after assigning = %+v, want the added font", got)
	}

	st, err = svc.RemoveFont("Gotham Medium")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pool) != 0 {
		t.Errorf("pool after remove = %+v", st.Pool)
	}
	got = roleLine(st, "title")
	if got.Source != "template-missing-override" || got.Override != "gotham medium" || got.Family != "Beleren Proxy" {
		t.Errorf("title after the font went = %+v, want the role's own font and the missing name", got)
	}
}

func TestAnAddedFontMustBeAFont(t *testing.T) {
	svc, _ := fontsService(t, fontdir.KindManaged)
	bad := filepath.Join(t.TempDir(), "Bad.ttf")
	if err := os.WriteFile(bad, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddFont(bad); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("bad upload: %v, want a bad request", err)
	}
	if _, err := svc.AddFont(filepath.Join(t.TempDir(), "missing.ttf")); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("missing file: %v, want a bad request", err)
	}
	if _, err := svc.RemoveFont("No Such Font Regular"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("removing an unknown font: %v, want not found", err)
	}
}

func TestAFolderTheAppDoesNotOwnIsReadOnly(t *testing.T) {
	svc, ws := fontsService(t, fontdir.KindCheckout)
	if err := os.MkdirAll(ws.Fonts.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Fonts.Path, "Theirs.otf"), engineFont(t, "Gotham-Medium.otf"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := svc.Fonts()
	if st.Writable || len(st.Pool) != 1 || st.Pool[0].Removable {
		t.Fatalf("status = %+v, want their font listed and not removable", st)
	}
	if _, err := svc.AddFont(writeFontFile(t, "Mine.otf", "Gotham-Medium.otf")); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("upload to a checkout: %v, want a conflict", err)
	}
	if _, err := svc.RemoveFont("Gotham Medium"); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("remove from a checkout: %v, want a conflict", err)
	}
}

func TestSymbolRevalidatesByTag(t *testing.T) {
	svc, _ := fontsService(t, fontdir.KindManaged)
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

func TestSymbolTagChangesWhenTheManaFontIsAssigned(t *testing.T) {
	svc, ws := fontsService(t, fontdir.KindManaged)
	_, before, err := svc.Symbol("W", 32, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddFont(writeFontFile(t, "Icons.otf", "Gotham-Medium.otf")); err != nil {
		t.Fatal(err)
	}
	assign(ws, "mana", "Gotham Medium")
	_, after, err := svc.Symbol("W", 32, before)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Error("a pip under an input keeps its tag after the mana font is replaced, so it would never redraw")
	}
}
