package template

import (
	"context"
	"errors"
	"testing"

	"github.com/odevine/mimic/engine/card"
)

var testSupports = Supports{Roles: []Role{RoleSingle}, Kinds: []Kind{KindStandard}}

func TestClassify(t *testing.T) {
	faces := func(front, back string) []card.Face {
		return []card.Face{{TypeLine: front}, {TypeLine: back}}
	}
	cases := []struct {
		name string
		card card.Data
		want []Shape
	}{
		{"instant", card.Data{Layout: "normal", TypeLine: "Instant"}, []Shape{{Role: RoleSingle, Kind: KindStandard}}},
		{"typed in by hand", card.Data{TypeLine: "Creature — Bear"}, []Shape{{Role: RoleSingle, Kind: KindStandard}}},
		{"planeswalker", card.Data{Layout: "normal", TypeLine: "Legendary Planeswalker — Jace"}, []Shape{{Role: RoleSingle, Kind: KindPlaneswalker}}},
		{"basic land", card.Data{Layout: "normal", TypeLine: "Basic Land — Forest"}, []Shape{{Role: RoleSingle, Kind: KindBasicLand}}},
		{"snow basic", card.Data{Layout: "normal", TypeLine: "Basic Snow Land — Island"}, []Shape{{Role: RoleSingle, Kind: KindBasicLand}}},
		{"nonbasic land", card.Data{Layout: "normal", TypeLine: "Land"}, []Shape{{Role: RoleSingle, Kind: KindStandard}}},
		{"saga by layout", card.Data{Layout: "saga", TypeLine: "Enchantment — Saga"}, []Shape{{Role: RoleSingle, Kind: KindSaga}}},
		{"saga typed in", card.Data{TypeLine: "Enchantment — Saga"}, []Shape{{Role: RoleSingle, Kind: KindSaga}}},
		{"class", card.Data{Layout: "class", TypeLine: "Enchantment — Class"}, []Shape{{Role: RoleSingle, Kind: KindClass}}},
		{"mutate", card.Data{Layout: "mutate", TypeLine: "Creature — Beast"}, []Shape{{Role: RoleSingle, Kind: KindMutate}}},
		{"token", card.Data{Layout: "token", TypeLine: "Token Creature — Goblin"}, []Shape{{Role: RoleSingle, Kind: KindToken}}},
		{"plane", card.Data{Layout: "planar", TypeLine: "Plane — Dominaria"}, []Shape{{Role: RoleSingle, Kind: KindPlane}}},
		{"split", card.Data{Layout: "split", Faces: faces("Instant", "Instant")}, []Shape{{Role: RoleSplit, Kind: KindStandard}}},
		{"fuse", card.Data{Layout: "split", Keywords: []string{"Fuse"}, Faces: faces("Instant", "Instant")}, []Shape{{Role: RoleSplit, Kind: KindStandard}}},
		{"aftermath by keyword", card.Data{Layout: "split", Keywords: []string{"Aftermath"}, Faces: faces("Sorcery", "Sorcery")}, []Shape{{Role: RoleAftermath, Kind: KindStandard}}},
		{
			"aftermath by reminder", card.Data{Layout: "split", Faces: []card.Face{{TypeLine: "Sorcery"}, {TypeLine: "Sorcery", OracleText: "Aftermath (Cast this spell only from your graveyard.)"}}},
			[]Shape{{Role: RoleAftermath, Kind: KindStandard}},
		},
		{"room", card.Data{Layout: "split", TypeLine: "Enchantment — Room // Enchantment — Room", Faces: faces("Enchantment — Room", "Enchantment — Room")}, []Shape{{Role: RoleSplit, Kind: KindRoom}}},
		{"adventure", card.Data{Layout: "adventure", Faces: faces("Creature — Human", "Sorcery — Adventure")}, []Shape{{Role: RoleAdventure, Kind: KindStandard}}},
		{
			"transform", card.Data{Layout: "transform", Faces: faces("Creature — Human Wizard", "Creature — Human Insect")},
			[]Shape{{Role: RoleTransformFront, Kind: KindStandard}, {Role: RoleTransformBack, Kind: KindStandard}},
		},
		{
			"planeswalker back", card.Data{Layout: "transform", Faces: faces("Legendary Creature — Human", "Legendary Planeswalker — Nissa")},
			[]Shape{{Role: RoleTransformFront, Kind: KindStandard}, {Role: RoleTransformBack, Kind: KindPlaneswalker}},
		},
		{
			"mdfc", card.Data{Layout: "modal_dfc", Faces: faces("Instant", "Land")},
			[]Shape{{Role: RoleMDFCFront, Kind: KindStandard}, {Role: RoleMDFCBack, Kind: KindStandard}},
		},
		{
			"battle", card.Data{Layout: "battle", Faces: faces("Battle — Siege", "Legendary Creature — Phyrexian")},
			[]Shape{{Role: RoleTransformFront, Kind: KindBattle}, {Role: RoleTransformBack, Kind: KindStandard}},
		},
		{"flip", card.Data{Layout: "flip", Faces: faces("Creature", "Creature")}, []Shape{{Role: RoleUnknown, Kind: KindUnknown}}},
		{"transform missing a face", card.Data{Layout: "transform"}, []Shape{{Role: RoleUnknown, Kind: KindUnknown}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(&c.card)
			if len(got) != len(c.want) {
				t.Fatalf("Classify = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("face %d = %v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestSupportsAllows(t *testing.T) {
	s := Supports{Roles: []Role{RoleTransformFront, RoleTransformBack}, Kinds: []Kind{KindPlaneswalker}}
	if !s.Allows(Shape{Role: RoleTransformBack, Kind: KindPlaneswalker}) {
		t.Error("a listed role and kind should be allowed")
	}
	if s.Allows(Shape{Role: RoleSingle, Kind: KindPlaneswalker}) {
		t.Error("a plain planeswalker should not match a transform-only template")
	}
	if s.Allows(Shape{Role: RoleTransformFront, Kind: KindStandard}) {
		t.Error("a standard face should not match a planeswalker-only template")
	}
}

func TestSupportsValidate(t *testing.T) {
	bad := []Supports{
		{},
		{Roles: []Role{RoleSingle}},
		{Roles: []Role{RoleUnknown}, Kinds: []Kind{KindStandard}},
		{Roles: []Role{RoleSingle}, Kinds: []Kind{"sticker"}},
	}
	for _, s := range bad {
		if s.validate() == nil {
			t.Errorf("validate(%+v) should fail", s)
		}
	}
	if err := testSupports.validate(); err != nil {
		t.Errorf("validate(testSupports) = %v", err)
	}
}

func TestGetRefusesUnsupportedFace(t *testing.T) {
	Register("fake-guard", "a fake template for tests", testSupports, func() Template { return fakeTemplate{name: "fake-guard"} })
	tmpl, err := Get("fake-guard")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := tmpl.Render(ctx, RenderRequest{Card: &card.Data{TypeLine: "Instant"}}); err != nil {
		t.Errorf("a standard card should render: %v", err)
	}

	_, err = tmpl.Render(ctx, RenderRequest{Card: &card.Data{TypeLine: "Legendary Planeswalker — Jace"}})
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Shape != (Shape{Role: RoleSingle, Kind: KindPlaneswalker}) {
		t.Errorf("a planeswalker should be refused with UnsupportedError, got %v", err)
	}

	dfc := &card.Data{Layout: "transform", Faces: []card.Face{{TypeLine: "Creature"}, {TypeLine: "Creature"}}}
	if _, err := tmpl.Render(ctx, RenderRequest{Card: dfc, Face: 2}); err == nil || errors.As(err, &unsupported) {
		t.Errorf("an out of range face should fail as out of range, got %v", err)
	}
	if _, err := tmpl.Render(ctx, RenderRequest{}); err == nil {
		t.Error("a request with no card should fail")
	}
}

func TestRegisterPanicsOnBadSupports(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Register with empty supports should panic")
		}
	}()
	Register("fake-bad-supports", "a fake template for tests", Supports{}, func() Template { return fakeTemplate{} })
}

func TestFaceCard(t *testing.T) {
	dfc := &card.Data{
		Name: "Delver of Secrets", Layout: "transform",
		Faces: []card.Face{{Name: "Delver of Secrets"}, {Name: "Insectile Aberration", ArtworkURL: "back.jpg"}},
	}
	if got := (RenderRequest{Card: dfc}).FaceCard(); got != dfc {
		t.Error("face 0 should be the card itself, so edits to it stick")
	}
	back := (RenderRequest{Card: dfc, Face: 1}).FaceCard()
	if back.Name != "Insectile Aberration" || back.ArtworkURL != "back.jpg" {
		t.Errorf("face 1 = %+v", back)
	}
	split := &card.Data{Name: "Fire // Ice", Layout: "split", Faces: []card.Face{{Name: "Fire"}, {Name: "Ice"}}}
	if got := (RenderRequest{Card: split, Face: 1}).FaceCard(); got != split {
		t.Error("a split card renders as one image, so any face is the whole card")
	}
}
