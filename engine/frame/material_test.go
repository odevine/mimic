package frame

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func TestDualRunsEveryPairTheWayThePrintedFramesDo(t *testing.T) {
	const (
		w, u, b, r, g = card.White, card.Blue, card.Black, card.Red, card.Green
	)
	cases := []struct {
		a, b card.Color
		run  string // left to right
		key  string // the image's name, in WUBRG order
	}{
		{w, u, "wu", "wu"},
		{u, b, "ub", "ub"},
		{b, r, "br", "br"},
		{r, g, "rg", "rg"},
		{g, w, "gw", "wg"},
		{w, b, "wb", "wb"},
		{u, r, "ur", "ur"},
		{b, g, "bg", "bg"},
		{r, w, "rw", "wr"},
		{g, u, "gu", "ug"},
	}
	for _, c := range cases {
		for _, in := range [][]card.Color{{c.a, c.b}, {c.b, c.a}} {
			m := Dual(in)
			if m.Kind() != MatRun || m.String() != c.run || m.Key() != c.key {
				t.Errorf("Dual(%v) = %v %q key %q, want a run %q key %q", in, m.Kind(), m.String(), m.Key(), c.run, c.key)
			}
		}
	}
}

func TestDualNeedsExactlyTwoColors(t *testing.T) {
	for _, in := range [][]card.Color{nil, {card.White}, {card.White, card.Blue, card.Black}} {
		if m := Dual(in); !m.IsNone() {
			t.Errorf("Dual(%v) = %q, want no material", in, m)
		}
	}
}

func TestRunKeepsTheOrderItIsGiven(t *testing.T) {
	m := Run(card.Green, card.White, card.Red)
	if m.Kind() != MatRun || m.String() != "gwr" || m.Key() != "wrg" {
		t.Errorf("Run = %v %q key %q, want a run gwr with key wrg", m.Kind(), m.String(), m.Key())
	}
	if got := m.Letters(); len(got) != 3 || got[0] != "g" || got[1] != "w" || got[2] != "r" {
		t.Errorf("Letters = %v, want g w r", got)
	}
	if one := Run(card.Red); one != Mono(card.Red) {
		t.Errorf("one color = %v, want a mono", one)
	}
	if none := Run(); !none.IsNone() {
		t.Errorf("no colors = %v, want no material", none)
	}
}

func TestMaterialKeysAndLetters(t *testing.T) {
	cases := []struct {
		name    string
		m       Material
		kind    MaterialKind
		key     string
		letters []string
	}{
		{"none", Material{}, MatNone, "", nil},
		{"mono", Mono(card.Blue), MatMono, "u", []string{"u"}},
		{"gold", Gold, MatGold, "gold", nil},
		{"artifact", Artifact, MatArtifact, "artifact", nil},
		{"colorless", Colorless, MatColorless, "colorless", nil},
		{"land", Land, MatLand, "land", nil},
		{"vehicle", Vehicle, MatVehicle, "vehicle", nil},
		{"named", Named("sunmoondfc"), MatNamed, "sunmoondfc", nil},
		{"an indicator's colors", ColorSet([]card.Color{card.Red, card.Black, card.Blue}), MatNamed, "ubr", nil},
	}
	for _, c := range cases {
		if c.m.Kind() != c.kind || c.m.Key() != c.key || len(c.m.Letters()) != len(c.letters) {
			t.Errorf("%s: kind %v key %q letters %v, want %v %q %v", c.name, c.m.Kind(), c.m.Key(), c.m.Letters(), c.kind, c.key, c.letters)
		}
		if c.m.IsNone() != (c.kind == MatNone) {
			t.Errorf("%s: IsNone = %v", c.name, c.m.IsNone())
		}
	}
	if !Named("").IsNone() || !ColorSet(nil).IsNone() {
		t.Error("an empty name or color set should be no material")
	}
}

func TestMaterialIsComparable(t *testing.T) {
	seen := map[Material]string{Dual([]card.Color{card.Green, card.White}): "gw"}
	if seen[Run(card.Green, card.White)] != "gw" {
		t.Error("two ways of making the same run should be the same map key")
	}
	if Run(card.Green, card.White) == Run(card.White, card.Green) {
		t.Error("runs in different orders are different materials")
	}
}
