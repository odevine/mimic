package batch

import (
	"image/png"
	"testing"

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
