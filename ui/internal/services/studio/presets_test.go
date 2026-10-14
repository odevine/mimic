package studio

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// source answers a lookup from a few cards, counting the searches it is asked
type source struct {
	cards   []*card.Data
	fail    error
	queries atomic.Int32
}

func (s *source) Search(_ context.Context, q string) ([]*card.Data, error) {
	s.queries.Add(1)
	if s.fail != nil {
		return nil, s.fail
	}
	var out []*card.Data
	for _, c := range s.cards {
		if strings.Contains(q, fmt.Sprintf("%q", c.Name)) && strings.Contains(q, "set:"+c.SetCode) && strings.Contains(q, "cn:"+c.CollectorNumber) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *source) FetchByName(context.Context, string) (*card.Data, error) { return nil, nil }

func withSource(svc *Service, src *source) {
	svc.resolve = func() cardlist.Resolver { return cardlist.APIResolver(src) }
}

func TestThePresetsAreSixRealPrintingsNamedBySetAndNumber(t *testing.T) {
	svc, _, _ := newService(t)
	got := svc.Presets()
	want := []Preset{
		{"monocolored", "Monocolored", "Mirran Spy", "mbs", "26"},
		{"two-colors", "Two Colors", "Consuming Aberration", "gtc", "152"},
		{"three-colors", "Three Colors", "Eerie Ultimatum", "iko", "184"},
		{"legendary", "Legendary Creature", "Norin the Wary", "tsp", "171"},
		{"artifact", "Artifact", "Sensei's Divining Top", "2x2", "314"},
		{"land", "Land", "Flooded Strand", "ktk", "233"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d presets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("preset %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAPresetIsLookedUpAsThePrintingItNamesAndKeepsItsArt(t *testing.T) {
	svc, _, _ := newService(t)
	spy := &card.Data{Name: "Mirran Spy", SetCode: "mbs", CollectorNumber: "26", TypeLine: "Creature - Human Rogue", Layout: "normal", ArtworkURL: "https://cards.example/mirran-spy.jpg", Colors: []card.Color{card.Blue}}
	other := &card.Data{Name: "Mirran Spy", SetCode: "mbs", CollectorNumber: "27", Layout: "normal"}
	src := &source{cards: []*card.Data{other, spy}}
	withSource(svc, src)

	got, err := svc.ResolvePreset("monocolored")
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.CollectorNumber != "26" || got.Data.ArtworkURL != spy.ArtworkURL {
		t.Errorf("card = %+v", got.Data)
	}
	if len(got.Shapes) != 1 || got.Shapes[0].String() != "standard (single)" {
		t.Errorf("shapes = %v", got.Shapes)
	}

	// Choosing it again does not ask again
	asked := src.queries.Load()
	if _, err := svc.ResolvePreset("monocolored"); err != nil {
		t.Fatal(err)
	}
	if src.queries.Load() != asked {
		t.Errorf("a second choice searched again, %d times", src.queries.Load()-asked)
	}
}

func TestAPresetThatCannotBeLookedUpSaysWhichAndWhy(t *testing.T) {
	svc, _, _ := newService(t)
	src := &source{fail: fmt.Errorf("scryfall answered 503")}
	withSource(svc, src)
	_, err := svc.ResolvePreset("land")
	if err == nil || apierr.KindOf(err) != apierr.BadGateway {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"Flooded Strand", "KTK 233", "503"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not say %q", err, want)
		}
	}
	// A failure is not remembered, so choosing again asks again and can succeed
	src.fail = nil
	src.cards = []*card.Data{{Name: "Flooded Strand", SetCode: "ktk", CollectorNumber: "233", TypeLine: "Land", Layout: "normal"}}
	if got, err := svc.ResolvePreset("land"); err != nil || got.Data.Name != "Flooded Strand" {
		t.Errorf("after the source came back: %v, %v", got, err)
	}
}

func TestAPresetThatScryfallDoesNotHaveIsAnError(t *testing.T) {
	svc, _, _ := newService(t)
	withSource(svc, &source{})
	_, err := svc.ResolvePreset("artifact")
	if err == nil || !strings.Contains(err.Error(), "Sensei's Divining Top") {
		t.Errorf("err = %v", err)
	}
}

func TestAnUnknownPresetIsRefused(t *testing.T) {
	svc, _, _ := newService(t)
	withSource(svc, &source{})
	if _, err := svc.ResolvePreset("instant"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("a preset that was removed gave %v", err)
	}
}
