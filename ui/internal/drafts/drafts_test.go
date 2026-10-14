package drafts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/templatetest"
)

func newStore(t *testing.T) *Store { return NewStore(filepath.Join(t.TempDir(), "drafts")) }

func valid(t *testing.T, s *Store, id string) {
	t.Helper()
	fsys, err := s.FS(id)
	if err != nil {
		t.Fatal(err)
	}
	if ds := author.Validate(fsys, author.Options{}); ds.HasErrors() {
		t.Fatalf("draft %s does not validate: %v", id, ds)
	}
}

func TestSlug(t *testing.T) {
	tests := map[string]string{
		"My frame":              "my-frame",
		"  Spaced   Out  ":      "spaced-out",
		"Ünïcode & Symbols!":    "n-code-symbols",
		"!!!":                   "template",
		"":                      "template",
		strings.Repeat("a", 90): strings.Repeat("a", maxSlugLen),
	}
	for in, want := range tests {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateMakesAValidDraftThatIsListed(t *testing.T) {
	s := newStore(t)
	sum, err := s.Create("My frame", 150)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ID != "my-frame" || sum.TemplateID != "user.my-frame" || sum.Name != "My frame" || sum.Problem != "" {
		t.Fatalf("summary = %+v", sum)
	}
	valid(t, s, sum.ID)
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].ID != sum.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestCreateGivesEachDraftAFolderOfItsOwn(t *testing.T) {
	s := newStore(t)
	var ids []string
	for range 3 {
		sum, err := s.Create("Same name", 100)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sum.ID)
	}
	if strings.Join(ids, ",") != "same-name,same-name-2,same-name-3" {
		t.Errorf("ids = %v", ids)
	}
	// Each carries the template id of its own folder, so two never clash once installed
	for _, id := range ids {
		sum, _ := s.Get(id)
		if sum.TemplateID != TemplateID(id) {
			t.Errorf("%s has template id %s", id, sum.TemplateID)
		}
	}
}

func TestCreateRefusesWhatCannotBeMade(t *testing.T) {
	s := newStore(t)
	for _, tt := range []struct {
		name string
		dpi  int
	}{{"", 0}, {"   ", 0}, {strings.Repeat("x", 81), 0}, {"Fine", 5000}} {
		if _, err := s.Create(tt.name, tt.dpi); apierr.KindOf(err) != apierr.BadRequest {
			t.Errorf("Create(%q, %d) = %v, want a bad request", tt.name, tt.dpi, err)
		}
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("a refused create left %+v", list)
	}
}

func TestForkCopiesATemplateAndCreditsIt(t *testing.T) {
	spec := templatetest.Spec{ID: "test.frame", Version: "1.2.3", Name: "Test frame"}
	dir := t.TempDir()
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	sum, err := s.Fork(template.NewFSAssetProvider(dir), template.BundleHeader{Version: "1.2.3", MinEngine: ""}, "My copy")
	if err != nil {
		t.Fatal(err)
	}
	if sum.TemplateID != "user.my-copy" || sum.Name != "My copy" || sum.ForkedFrom != "test.frame 1.2.3" {
		t.Fatalf("summary = %+v", sum)
	}
	valid(t, s, sum.ID)

	fsys, _ := s.FS(sum.ID)
	tm, ds, err := author.NewDraft(fsys, 64<<20).Open(author.Options{})
	if err != nil {
		t.Fatalf("%v: %v", err, ds)
	}
	if tm.ID() != "user.my-copy" {
		t.Errorf("template id = %s", tm.ID())
	}
	credits := tm.Manifest().Credits
	if len(credits) != 1 || credits[0].Note != "Forked from test.frame 1.2.3" {
		t.Errorf("credits = %+v", credits)
	}
}

func TestForkKeepsTheOriginalsCreditsAndLicense(t *testing.T) {
	spec := templatetest.Spec{ID: "test.frame"}
	b, err := spec.Pack()
	if err != nil {
		t.Fatal(err)
	}
	m := b.Manifest
	m.License = "CC BY 4.0"
	m.Author.Name = "Original Author"
	m.Author.URL = "https://example.com/author"
	m.Credits = append(m.Credits[:0], m.Credits...)
	raw, err := jsonOf(m)
	if err != nil {
		t.Fatal(err)
	}
	p := template.NewMemoryAssets(raw, b.Assets)

	s := newStore(t)
	sum, err := s.Fork(p, template.BundleHeader{Version: "0.1.0"}, "Derived")
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := s.Meta(sum.ID)
	if meta.LockedCredits != 1 || !meta.LicenseLocked {
		t.Errorf("meta = %+v, want the credit of the original and the license locked", meta)
	}
	fsys, _ := s.FS(sum.ID)
	tm, _, err := author.NewDraft(fsys, 64<<20).Open(author.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := tm.Manifest()
	if got.License != "CC BY 4.0" {
		t.Errorf("license = %q", got.License)
	}
	if len(got.Credits) != 1 || got.Credits[0].Name != "Original Author" || got.Credits[0].URL != "https://example.com/author" {
		t.Errorf("credits = %+v, want the original's author named", got.Credits)
	}
	if got.Author.Name != "" {
		t.Errorf("author = %+v, want the fork to name no author", got.Author)
	}
}

func TestRenameChangesTheNameAndNothingElse(t *testing.T) {
	s := newStore(t)
	sum, _ := s.Create("First", 100)
	renamed, err := s.Rename(sum.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Second" || renamed.ID != sum.ID || renamed.TemplateID != sum.TemplateID {
		t.Errorf("renamed = %+v", renamed)
	}
	valid(t, s, sum.ID)
	if _, err := s.Rename(sum.ID, " "); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a blank name gave %v", err)
	}
	if _, err := s.Rename("no-such", "X"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("renaming a missing draft gave %v", err)
	}
}

func TestDeleteRemovesTheDraft(t *testing.T) {
	s := newStore(t)
	sum, _ := s.Create("Gone", 100)
	if err := s.Delete(sum.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("list = %+v", list)
	}
	if err := s.Delete(sum.ID); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("deleting twice gave %v", err)
	}
}

func TestIDsCannotReachOutsideTheStore(t *testing.T) {
	s := newStore(t)
	outside := filepath.Join(filepath.Dir(s.dir), "precious")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../precious", "..", "a/b", "", "UPPER", "-lead", `a\b`} {
		if err := s.Delete(id); apierr.KindOf(err) != apierr.BadRequest {
			t.Errorf("Delete(%q) = %v, want a bad request", id, err)
		}
		if _, err := s.FS(id); err == nil {
			t.Errorf("FS(%q) opened a folder", id)
		}
		if _, err := s.Rename(id, "X"); err == nil {
			t.Errorf("Rename(%q) worked", id)
		}
		if err := s.SaveMeta(id, Meta{}); err == nil {
			t.Errorf("SaveMeta(%q) worked", id)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("a folder outside the store was touched: %v", err)
	}
}

func TestMetaIsKeptBesideTheDraft(t *testing.T) {
	s := newStore(t)
	sum, _ := s.Create("Meta", 100)
	if m, err := s.Meta(sum.ID); err != nil || m.TestCard != nil {
		t.Fatalf("fresh meta = %+v, %v", m, err)
	}
	want := Meta{TestCard: &card.Data{Name: "Bolt", TypeLine: "Instant"}, Face: 1}
	if err := s.SaveMeta(sum.ID, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Meta(sum.ID)
	if err != nil || got.TestCard == nil || got.TestCard.Name != "Bolt" || got.Face != 1 {
		t.Fatalf("meta = %+v, %v", got, err)
	}
	valid(t, s, sum.ID)

	// A damaged file is not worth failing a draft over
	if err := os.WriteFile(filepath.Join(s.dir, sum.ID, metaFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m, err := s.Meta(sum.ID); err != nil || m.TestCard != nil {
		t.Errorf("damaged meta = %+v, %v", m, err)
	}
}

func TestListShowsADraftThatCannotBeReadAndSkipsStaging(t *testing.T) {
	s := newStore(t)
	good, _ := s.Create("Good", 100)
	if err := os.MkdirAll(filepath.Join(s.dir, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, ".tmp-half"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	for _, sum := range list {
		switch sum.ID {
		case good.ID:
			if sum.Problem != "" {
				t.Errorf("good draft has a problem: %+v", sum)
			}
		case "broken":
			if sum.Problem == "" || sum.Name != "broken" {
				t.Errorf("broken draft = %+v", sum)
			}
		default:
			t.Errorf("unexpected entry %+v", sum)
		}
	}
	if err := s.Delete("broken"); err != nil {
		t.Errorf("a broken draft could not be deleted: %v", err)
	}
}

func TestAnEmptyStoreListsNothing(t *testing.T) {
	list, err := newStore(t).List()
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("list = %#v, %v", list, err)
	}
}

func jsonOf(v any) ([]byte, error) { return json.Marshal(v) }

func TestMetaKeepsWhatIsLockedAndWhatSnaps(t *testing.T) {
	s := newStore(t)
	sum, _ := s.Create("Marks", 100)
	want := Meta{Locked: []string{"normal/layer:background"}, SnapTo: []string{"normal/text:title", "normal/art:0"}}
	if err := s.SaveMeta(sum.ID, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Meta(sum.ID)
	if err != nil || !reflect.DeepEqual(got.Locked, want.Locked) || !reflect.DeepEqual(got.SnapTo, want.SnapTo) {
		t.Fatalf("meta = %+v, %v", got, err)
	}
	valid(t, s, sum.ID)
}

func TestPutFileMakesTheFoldersAndRemoveFileTidiesThem(t *testing.T) {
	s := newStore(t)
	sum, _ := s.Create("Files", 100)
	if err := s.PutFile(sum.ID, "parts/glow/any.png", []byte("one")); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(s.dir, sum.ID, filepath.FromSlash(name)))
		if err != nil {
			return ""
		}
		return string(raw)
	}
	if read("parts/glow/any.png") != "one" {
		t.Fatal("the file was not written")
	}
	if err := s.PutFile(sum.ID, "parts/glow/any.png", []byte("two")); err != nil || read("parts/glow/any.png") != "two" {
		t.Fatalf("the file was not replaced: %v", err)
	}
	if err := s.RemoveFile(sum.ID, "parts/glow/any.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, sum.ID, "parts", "glow")); !os.IsNotExist(err) {
		t.Errorf("the empty folder was left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, sum.ID, "parts", "background")); err != nil {
		t.Errorf("a folder that still holds a file went too: %v", err)
	}
	if err := s.RemoveFile(sum.ID, "parts/glow/any.png"); err == nil {
		t.Error("removed a file that is not there")
	}
	for _, name := range []string{"../x.png", "/x.png", "", "."} {
		if err := s.PutFile(sum.ID, name, nil); err == nil {
			t.Errorf("put %q was allowed", name)
		}
		if err := s.RemoveFile(sum.ID, name); err == nil {
			t.Errorf("remove %q was allowed", name)
		}
	}
	if err := s.PutFile("nope", "a.png", nil); err == nil {
		t.Error("put into a draft that is not there")
	}
	valid(t, s, sum.ID)
}
