package fonts

import (
	"testing"
)

func TestResolverUsesTheRolesOwnFont(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	res := r.Inspect(Body)
	if res.Source != SourceTemplate || res.Family != "Plantin MT Pro" || res.Path != "embedded/m15/plantin-body.ttf" {
		t.Errorf("Body = %+v, want the built-in Plantin as the template's font", res)
	}
	if res.Override != "" || res.Err != nil {
		t.Errorf("Body reported an override or error: %+v", res)
	}
	if r.Font(Body).Fallback() {
		t.Error("Body fell back to the bitmap face")
	}
}

func TestResolverAppliesAnOverrideFromThePool(t *testing.T) {
	dir := t.TempDir()
	path := writeFont(t, dir, "gotham/G.otf", gothamMedium)
	pool := NewPool(dir)

	r := NewResolver(nil, pool, map[string]string{"title": "gotham medium"})
	res := r.Inspect(Title)
	if res.Source != SourceOverride || res.Path != path || res.Family != "Gotham" || res.Override != "gotham medium" {
		t.Errorf("Title = %+v, want the pool's Gotham", res)
	}
	// A role with no assignment keeps its own font.
	if got := r.Inspect(Body).Source; got != SourceTemplate {
		t.Errorf("Body source = %s, want template", got)
	}
}

func TestResolverFallsBackFromAMissingOverride(t *testing.T) {
	r := NewResolver(nil, NewPool(t.TempDir()), map[string]string{"title": "No Such Font Regular"})
	res := r.Inspect(Title)
	if res.Source != SourceMissingOverride || res.Override != "No Such Font Regular" {
		t.Errorf("Title = %+v, want a missing override", res)
	}
	if res.Family != "Beleren Proxy" || r.Font(Title).Fallback() {
		t.Errorf("a missing override should draw the role's own font, got %q", res.Family)
	}
	// With no pool at all the name cannot be found either.
	if got := NewResolver(nil, nil, map[string]string{"title": "x"}).Inspect(Title).Source; got != SourceMissingOverride {
		t.Errorf("source with no pool = %s", got)
	}
}

func TestResolverCustomRolesTakeOverrides(t *testing.T) {
	raw := embeddedBytes(t, plantinItalic)
	cat, err := NewCatalog(
		map[string]FontDef{"script": {File: "font/S.ttf"}},
		map[string]RoleDef{"saga-chapter": {Font: "script"}},
		memOpen(map[string][]byte{"font/S.ttf": raw}),
	)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFont(t, dir, "g.otf", gothamMedium)

	plain := NewResolver(cat, nil, nil).Inspect("saga-chapter")
	if plain.Source != SourceTemplate || plain.Style != "Italic" || plain.Path != "" {
		t.Errorf("saga-chapter = %+v, want the bundled italic with no path", plain)
	}
	over := NewResolver(cat, NewPool(dir), map[string]string{"saga-chapter": "Gotham Medium"}).Inspect("saga-chapter")
	if over.Source != SourceOverride || over.Family != "Gotham" {
		t.Errorf("saga-chapter override = %+v", over)
	}
}

func TestResolverKnowsRolesAndFallsBackForTheRest(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	if role, ok := r.Role("small-caps"); !ok || role != SmallCaps {
		t.Errorf("Role(small-caps) = %v, %v", role, ok)
	}
	if _, ok := r.Role("saga-chapter"); ok {
		t.Error("an undefined role was known")
	}
	if res := r.Inspect("saga-chapter"); res.Source != SourceFallback {
		t.Errorf("undefined role source = %s, want fallback", res.Source)
	}
	if !r.Font("saga-chapter").Fallback() {
		t.Error("an undefined role should draw the bitmap face")
	}
}

func TestResolverRemembersEachRole(t *testing.T) {
	r := NewResolver(nil, nil, nil)
	a, b := r.Font(Body), r.Font(Body)
	if a.font != b.font {
		t.Error("a role resolved to different fonts within one resolver")
	}
}

func TestResolverCopiesItsOverrides(t *testing.T) {
	over := map[string]string{"title": "Gotham Medium"}
	r := NewResolver(nil, nil, over)
	over["title"] = "changed"
	if got := r.Inspect(Title).Override; got != "Gotham Medium" {
		t.Errorf("override = %q, want the one the resolver was made with", got)
	}
}
