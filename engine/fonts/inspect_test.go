package fonts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRoleFont copies an embedded font into role's folder under dir as name
func writeRoleFont(t *testing.T, dir string, role Role, name, embeddedFile string) string {
	t.Helper()
	src, err := embedded.ReadFile(embeddedFile)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, roleDir[role])
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRolesCoverEveryFolder(t *testing.T) {
	roles := Roles()
	if len(roles) != len(roleDir) {
		t.Fatalf("Roles() lists %d roles, want %d", len(roles), len(roleDir))
	}
	for _, info := range roles {
		if info.Folder != roleDir[info.Role] {
			t.Errorf("%s folder = %q, want %q", info.Name, info.Folder, roleDir[info.Role])
		}
		if got, ok := ParseRole(info.Name); !ok || got != info.Role {
			t.Errorf("Roles() name %q does not parse back to its role", info.Name)
		}
	}
	if roles[4].Name != "small-caps" || roles[4].Builtin != "mimic-m15-small-caps" || roles[4].Label == "" {
		t.Errorf("small-caps entry = %+v, want its own built-in font and a label", roles[4])
	}
}

func TestInspectSources(t *testing.T) {
	dir := t.TempDir()
	path := writeRoleFont(t, dir, Title, "Mine.ttf", "embedded/m15/plantin-body.ttf")

	user := Inspect(Title, dir)
	if user.Source != SourceUser || user.Path != path {
		t.Errorf("Title = %s %q, want user %q", user.Source, user.Path, path)
	}
	if user.Family == "" || user.Style != "Regular" {
		t.Errorf("Title names = %q %q, want a family and Regular from the name table", user.Family, user.Style)
	}
	if def := Inspect(Body, dir); def.Source != SourceDefault || def.Family == "" {
		t.Errorf("Body = %s %q, want the built-in default", def.Source, def.Family)
	}
	if def := Inspect(Info, dir); def.Source != SourceDefault || def.Path != "embedded/m15/Gotham-Medium.otf" {
		t.Errorf("Info = %s %q, want its own built-in font", def.Source, def.Path)
	}
	if f := Inspect(Role("no-such-role"), dir); f.Source != SourceFallback {
		t.Errorf("unknown role source = %s, want fallback", f.Source)
	}
}

func TestInspectReportsRejectedAndIgnored(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, roleDir[Body])
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(folder, "a-broken.ttf")
	if err := os.WriteFile(bad, []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := writeRoleFont(t, dir, Body, "b-extra.ttf", "embedded/m15/plantin-body.ttf")

	res := Inspect(Body, dir)
	if res.Source != SourceDefault {
		t.Errorf("source = %s, want default after a rejected file", res.Source)
	}
	if res.Rejected != bad || res.Err == nil {
		t.Errorf("rejected = %q err = %v, want %q with an error", res.Rejected, res.Err, bad)
	}
	if len(res.Ignored) != 1 || res.Ignored[0] != extra {
		t.Errorf("ignored = %v, want [%s]", res.Ignored, extra)
	}
}

func TestReplacedFileIsReparsed(t *testing.T) {
	dir := t.TempDir()
	path := writeRoleFont(t, dir, Title, "Font.ttf", "embedded/m15/plantin-body.ttf")
	if got := Inspect(Title, dir).Style; got != "Regular" {
		t.Fatalf("style = %q, want Regular", got)
	}
	writeRoleFont(t, dir, Title, "Font.ttf", "embedded/m15/plantin-italic.ttf")
	// Some filesystems keep coarse modification times, so force a distinct one
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if got := Inspect(Title, dir).Style; got != "Italic" {
		t.Errorf("style after replacing = %q, want Italic", got)
	}
}
