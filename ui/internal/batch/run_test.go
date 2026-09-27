package batch

import (
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

func TestFailedReturnsOnlyFailures(t *testing.T) {
	rows := []Row{{Name: "Good"}, {Name: "Bad"}, {Name: "Also Bad"}}
	r := New(rows, Options{ID: "run-1", OutDir: t.TempDir(), Label: "deck"})
	r.cards[0].Status = StatusDone
	// Two cards failed, as an art download blip would leave them
	r.cards[1].Status, r.cards[2].Status = StatusFailed, StatusFailed

	failed := r.Failed()
	if len(failed) != 2 || failed[0].Name != "Bad" || failed[1].Name != "Also Bad" {
		t.Errorf("failed = %+v", failed)
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
