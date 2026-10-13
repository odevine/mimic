package fonts

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestNilCatalogHasTheStandardRoles(t *testing.T) {
	var c *Catalog
	roles := c.Roles()
	if len(roles) != 6 {
		t.Fatalf("%d roles, want the 6 standard ones", len(roles))
	}
	for i, want := range []Role{Title, Body, BodyItalic, Mana, SmallCaps, Info} {
		r := roles[i]
		if r.Role != want || r.Label == "" || r.Family == "" || r.Builtin == "" || r.Bundled {
			t.Errorf("roles[%d] = %+v, want %s with a label and its built-in font", i, r, want)
		}
	}
	if roles[4].Builtin != "mimic-m15-small-caps" {
		t.Errorf("small-caps builtin = %q", roles[4].Builtin)
	}
	if !c.Knows(Info) || c.Knows(Role("saga-chapter")) {
		t.Error("a nil catalog should know exactly the standard roles")
	}
}

func TestCatalogBundlesFontsAndDefinesRoles(t *testing.T) {
	raw := embeddedBytes(t, plantinItalic)
	files := map[string][]byte{"font/Script.ttf": raw}
	c, err := NewCatalog(
		map[string]FontDef{"script": {File: "font/Script.ttf", SHA256: sum(raw)}},
		map[string]RoleDef{
			"saga-chapter": {Label: "Chapter text", Font: "script"},
			"saga-name":    {Builtin: "mimic-m15-title"},
			"body":         {Label: "Rules", Font: "script"},
		},
		memOpen(files),
	)
	if err != nil {
		t.Fatal(err)
	}
	roles := c.Roles()
	var names []string
	for _, r := range roles {
		names = append(names, string(r.Role))
	}
	if got := strings.Join(names, ","); got != "title,body,body-italic,mana,small-caps,info,saga-chapter,saga-name" {
		t.Errorf("roles = %s", got)
	}
	byName := map[Role]RoleInfo{}
	for _, r := range roles {
		byName[r.Role] = r
	}
	if r := byName["saga-chapter"]; !r.Bundled || r.Family != "Plantin MT Pro" || r.Style != "Italic" || r.Label != "Chapter text" {
		t.Errorf("saga-chapter = %+v", r)
	}
	if r := byName["saga-name"]; r.Bundled || r.Builtin != "mimic-m15-title" || r.Label != "saga-name" {
		t.Errorf("saga-name = %+v, want the built-in title font labelled by its name", r)
	}
	if r := byName[Body]; !r.Bundled || r.Label != "Rules" {
		t.Errorf("body = %+v, want the template's own definition", r)
	}
	if r := byName[Title]; r.Bundled || r.Builtin != "mimic-m15-title" {
		t.Errorf("title = %+v, want the standard built-in", r)
	}
}

func TestCatalogDeclaredNamesWinOverTheNameTable(t *testing.T) {
	raw := embeddedBytes(t, plantinRegular)
	c, err := NewCatalog(
		map[string]FontDef{"f": {File: "font/F.ttf", Family: "House Serif", Style: "Book"}},
		map[string]RoleDef{"x": {Font: "f"}},
		memOpen(map[string][]byte{"font/F.ttf": raw}),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Roles()[6]
	if got.Family != "House Serif" || got.Style != "Book" {
		t.Errorf("names = %q %q, want the declared ones", got.Family, got.Style)
	}
}

func TestCatalogRefusesWhatCannotBeUsed(t *testing.T) {
	good := embeddedBytes(t, plantinRegular)
	open := memOpen(map[string][]byte{
		"font/Good.ttf": good,
		"font/Junk.ttf": []byte("not a font"),
	})
	cases := []struct {
		name  string
		fonts map[string]FontDef
		roles map[string]RoleDef
		want  string
	}{
		{"missing file", map[string]FontDef{"f": {File: "font/Gone.ttf"}}, nil, "font \"f\""},
		{"not a font", map[string]FontDef{"f": {File: "font/Junk.ttf"}}, nil, "is not a font"},
		{"wrong hash", map[string]FontDef{"f": {File: "font/Good.ttf", SHA256: strings.Repeat("0", 64)}}, nil, "does not match its sha256"},
		{"unknown font id", nil, map[string]RoleDef{"r": {Font: "nope"}}, "does not bundle"},
		{"unknown builtin", nil, map[string]RoleDef{"r": {Builtin: "nope"}}, "does not have"},
		{"both", map[string]FontDef{"f": {File: "font/Good.ttf"}}, map[string]RoleDef{"r": {Font: "f", Builtin: "mimic-m15-body"}}, "both"},
		{"neither", nil, map[string]RoleDef{"r": {}}, "names no font"},
	}
	for _, tc := range cases {
		c, err := NewCatalog(tc.fonts, tc.roles, open)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", tc.name, err, tc.want)
		}
		if c != nil {
			t.Errorf("%s: a catalog came back with the error", tc.name)
		}
	}
}

func TestCatalogRefusesAnOversizedFont(t *testing.T) {
	big := make([]byte, MaxFontBytes+1)
	_, err := NewCatalog(map[string]FontDef{"f": {File: "font/Big.ttf"}}, nil, memOpen(map[string][]byte{"font/Big.ttf": big}))
	if err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("err = %v, want the size limit", err)
	}
}

func TestBundledFontsAreParsedOncePerContent(t *testing.T) {
	raw := embeddedBytes(t, gothamMedium)
	mk := func(file string) *Catalog {
		c, err := NewCatalog(
			map[string]FontDef{"f": {File: file}},
			map[string]RoleDef{"r": {Font: "f"}},
			memOpen(map[string][]byte{file: raw}),
		)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, _ := mk("font/One.otf").own("r")
	b, _ := mk("font/Two.otf").own("r")
	if a.font != b.font {
		t.Error("the same bytes under two names were parsed twice")
	}
}
