package testcards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "testcards.json")
	return NewStore(path), path
}

func TestACardIsKeptWithItsFieldsAndComesBackInTheOrderItWasSaved(t *testing.T) {
	s, path := newStore(t)
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("a store with no file lists %v, %v", list, err)
	}
	a, err := s.Add("Long title", card.Data{Name: "A very long name", OracleText: "Words."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Add("Flip side", card.Data{Name: "Back"}, 1)
	if a.ID != "user-long-title" || b.ID != "user-flip-side" || !IsID(a.ID) {
		t.Fatalf("ids = %q, %q", a.ID, b.ID)
	}
	// A new store over the same file is what the next launch sees
	list, err := NewStore(path).List()
	if err != nil || len(list) != 2 || list[0].Card.OracleText != "Words." || list[1].Face != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	got, err := s.Get(b.ID)
	if err != nil || got.Card.Name != "Back" {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

func TestALabelIsTidiedAndAnotherCardCannotTakeIt(t *testing.T) {
	s, _ := newStore(t)
	a, err := s.Add("  Many   words  ", card.Data{Name: "A"}, 0)
	if err != nil || a.Label != "Many words" {
		t.Fatalf("label = %q, %v", a.Label, err)
	}
	if _, err := s.Add("MANY words", card.Data{Name: "B"}, 0); err == nil || apierr.KindOf(err) != apierr.Conflict {
		t.Fatalf("a repeated label = %v", err)
	}
	for _, label := range []string{"", "   ", strings.Repeat("x", MaxLabel+1)} {
		if _, err := s.Add(label, card.Data{Name: "C"}, 0); err == nil || apierr.KindOf(err) != apierr.BadRequest {
			t.Fatalf("label %q = %v", label, err)
		}
	}
}

func TestLabelsThatMakeTheSameSlugGetDistinctIds(t *testing.T) {
	s, _ := newStore(t)
	a, _ := s.Add("Cat!", card.Data{Name: "A"}, 0)
	b, _ := s.Add("Cat?", card.Data{Name: "B"}, 0)
	c, _ := s.Add("Cat.", card.Data{Name: "C"}, 0)
	if a.ID != "user-cat" || b.ID != "user-cat-2" || c.ID != "user-cat-3" {
		t.Fatalf("ids = %q %q %q", a.ID, b.ID, c.ID)
	}
}

func TestOnlyAFixedNumberOfCardsAreKept(t *testing.T) {
	s, _ := newStore(t)
	for i := 0; i < Max; i++ {
		if _, err := s.Add("Card "+strings.Repeat("a", i+1), card.Data{Name: "X"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Add("One more", card.Data{Name: "X"}, 0); err == nil || apierr.KindOf(err) != apierr.Conflict {
		t.Fatalf("one over the limit = %v", err)
	}
}

func TestARemovedCardIsGoneAndAnUnknownOneIsAnError(t *testing.T) {
	s, _ := newStore(t)
	a, _ := s.Add("One", card.Data{Name: "A"}, 0)
	b, _ := s.Add("Two", card.Data{Name: "B"}, 0)
	if err := s.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list = %+v", list)
	}
	if err := s.Remove(a.ID); err == nil || apierr.KindOf(err) != apierr.NotFound {
		t.Fatalf("removing it again = %v", err)
	}
	if _, err := s.Get(a.ID); err == nil {
		t.Fatal("a removed card was found")
	}
}

func TestAFileThatIsNotReadableIsReportedAndNotOverwritten(t *testing.T) {
	s, path := newStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("a broken file was listed")
	}
	if _, err := s.Add("One", card.Data{Name: "A"}, 0); err == nil {
		t.Fatal("a card was saved over a file that could not be read")
	}
	if raw, _ := os.ReadFile(path); string(raw) != "{not json" {
		t.Fatalf("the file was changed to %q", raw)
	}
}

func TestAStoreWithNoPathSaysSo(t *testing.T) {
	if _, err := NewStore("").Add("One", card.Data{Name: "A"}, 0); err == nil || apierr.KindOf(err) != apierr.Internal {
		t.Fatalf("add = %v", err)
	}
}
