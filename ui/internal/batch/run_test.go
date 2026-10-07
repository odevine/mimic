package batch

import (
	"image/png"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
)

func TestExpandFacesKeepsOneRowForOneFace(t *testing.T) {
	rows := ExpandFaces([]Row{
		{Qty: 1, Base: card.Data{Name: "Grizzly Bears", TypeLine: "Creature"}},
		{Base: card.Data{Name: "Meld", Layout: "meld"}},
	})
	if len(rows) != 2 || rows[0].Face != 0 || rows[1].Face != 0 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestRetryTakesOnlyFailures(t *testing.T) {
	rows := []Row{{Name: "Good"}, {Name: "Bad"}, {Name: "Also Bad"}}
	r := New(rows, Options{ID: "run-1", OutDir: t.TempDir(), Label: "deck"})
	r.cards[0].Status = StatusDone
	// Two cards failed, as an art download blip would leave them
	r.cards[1].Status, r.cards[2].Status = StatusFailed, StatusFailed

	failed, project := r.Retry()
	if len(failed) != 2 || failed[0].Name != "Bad" || failed[1].Name != "Also Bad" || project != nil {
		t.Errorf("failed = %+v, project = %v", failed, project)
	}
	if _, ok := r.File(1); ok {
		t.Error("a failed card has a file")
	}
}

func TestStopMarksARunStopped(t *testing.T) {
	r := New([]Row{{Name: "A"}}, Options{ID: "run-1"})
	if !r.Active() {
		t.Fatal("a new run should be active")
	}
	r.Stop()
	if !r.View().Stopped || r.ctx.Err() == nil {
		t.Error("Stop did not mark the run stopped and cancel it")
	}
}

func TestConcurrency(t *testing.T) {
	cases := []struct {
		name          string
		setting, auto int
		want          int
	}{
		{"automatic takes the machine's count", 0, 9, 9},
		{"a negative setting is automatic too", -3, 5, 5},
		{"a chosen count wins over automatic", 3, 9, 3},
		{"a chosen count may exceed automatic", 12, 4, 12},
		{"a chosen count stops at the maximum", 99, 4, MaxConcurrency},
		{"automatic stops at the maximum", 0, 99, MaxConcurrency},
		{"automatic never goes below one", 0, 0, 1},
	}
	for _, c := range cases {
		if got := Concurrency(c.setting, c.auto); got != c.want {
			t.Errorf("%s: Concurrency(%d, %d) = %d, want %d", c.name, c.setting, c.auto, got, c.want)
		}
	}
}

func TestCompression(t *testing.T) {
	if got := Compression("fast"); got != png.BestSpeed {
		t.Errorf("fast = %d, want BestSpeed", got)
	}
	for _, s := range []string{"", "balanced", "other"} {
		if got := Compression(s); got != png.DefaultCompression {
			t.Errorf("%q = %d, want the default", s, got)
		}
	}
}

func TestFormat(t *testing.T) {
	for _, c := range []struct{ setting, format, ext string }{
		{"", FormatJPEG, ".jpg"}, {"jpeg", FormatJPEG, ".jpg"}, {"bmp", FormatJPEG, ".jpg"}, {"png", FormatPNG, ".png"},
	} {
		if got := ParseFormat(c.setting); got != c.format || Ext(got) != c.ext {
			t.Errorf("%q = %q%s, want %q%s", c.setting, got, Ext(got), c.format, c.ext)
		}
	}
}

func TestProjectRunIsAlwaysPNG(t *testing.T) {
	rows := []Row{{Qty: 1, Base: card.Data{Name: "A"}}}
	p, err := PlanProject(rows, ProjectSpec{CardbackName: "Back"})
	if err != nil {
		t.Fatal(err)
	}
	r := New(rows, Options{Format: FormatJPEG, Project: p})
	if r.opts.Format != FormatPNG || filepath.Ext(r.files[0]) != ".png" {
		t.Errorf("a project run is %q as %q, want PNG", r.opts.Format, r.files[0])
	}
}

// finish moves card i through to a status the way the workers do
func finish(r *Run, i int, status string) {
	r.update(i, "", func(c *Card) { c.Status = status })
}

func warmRun(cards, workers int) *Run {
	rows := make([]Row, cards)
	return New(rows, Options{Concurrency: workers})
}

func TestWarmupEndsWithTheFirstWaveOfRenderedCards(t *testing.T) {
	r := warmRun(6, 3)
	if v := r.View(); v.Warmup != 0 || v.WarmupCards != 0 {
		t.Fatalf("a run that has not started has warm-up %d ms over %d cards", v.Warmup, v.WarmupCards)
	}
	finish(r, 0, StatusDone)
	finish(r, 1, StatusFailed) // a failure was rendered too
	if v := r.View(); v.Warmup != 0 {
		t.Fatalf("warm-up ended after 2 of 3 cards: %+v", v)
	}
	time.Sleep(3 * time.Millisecond)
	finish(r, 2, StatusDone)
	v := r.View()
	if v.Warmup < 3 || v.WarmupCards != 3 {
		t.Fatalf("warm-up = %d ms over %d cards, want at least 3 ms over 3 cards", v.Warmup, v.WarmupCards)
	}

	// Later cards do not move it
	time.Sleep(3 * time.Millisecond)
	finish(r, 3, StatusDone)
	finish(r, 4, StatusDone)
	if again := r.View(); again.Warmup != v.Warmup || again.WarmupCards != v.WarmupCards {
		t.Errorf("warm-up moved from %+v to %+v", v.Warmup, again.Warmup)
	}
}

func TestWarmupIgnoresCardsThatWereNeverRendered(t *testing.T) {
	r := warmRun(4, 2)
	finish(r, 0, StatusSkipped)
	finish(r, 1, StatusUnsupported)
	finish(r, 2, StatusDone)
	if v := r.View(); v.Warmup != 0 {
		t.Fatalf("skipped and unsupported cards ended the warm-up: %+v", v)
	}
	finish(r, 3, StatusDone)
	if v := r.View(); v.Warmup == 0 || v.WarmupCards != 2 {
		t.Errorf("warm-up = %d ms over %d cards, want it set over 2", v.Warmup, v.WarmupCards)
	}
}

func TestWarmupOfARunSmallerThanItsWorkersIsTheWholeRun(t *testing.T) {
	r := warmRun(2, 8)
	finish(r, 0, StatusDone)
	if v := r.View(); v.Warmup != 0 {
		t.Fatalf("warm-up ended with a card still to render: %+v", v)
	}
	finish(r, 1, StatusDone)
	if v := r.View(); v.Warmup == 0 || v.WarmupCards != 2 {
		t.Errorf("warm-up = %d ms over %d cards, want it set over both", v.Warmup, v.WarmupCards)
	}
}

func TestWarmupNeverSetWhenNothingRenders(t *testing.T) {
	r := warmRun(2, 2)
	finish(r, 0, StatusUnsupported)
	finish(r, 1, StatusSkipped)
	if v := r.View(); v.Warmup != 0 || v.WarmupCards != 0 {
		t.Errorf("a run that rendered nothing has warm-up %+v", v)
	}
}

func TestTheEventThatEndsTheWarmupCarriesIt(t *testing.T) {
	var mu sync.Mutex
	var events []Event
	r := New(make([]Row, 5), Options{Concurrency: 2, Emit: func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}})
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		finish(r, i, StatusDone)
	}
	var carrying []Event
	for _, e := range events {
		if e.Warmup != 0 || e.WarmupCards != 0 {
			carrying = append(carrying, e)
		}
	}
	if len(carrying) != 1 {
		t.Fatalf("%d events carry the warm-up, want exactly one", len(carrying))
	}
	if v := r.View(); carrying[0].Warmup != v.Warmup || carrying[0].WarmupCards != v.WarmupCards || carrying[0].WarmupCards != 2 {
		t.Errorf("event has %d ms over %d cards, view has %d ms over %d cards, want 2 cards", carrying[0].Warmup, carrying[0].WarmupCards, v.Warmup, v.WarmupCards)
	}
}
