package list

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

// noCards is a card source that knows no cards
type noCards struct{}

func (noCards) Search(context.Context, string) ([]*card.Data, error)    { return nil, nil }
func (noCards) FetchByName(context.Context, string) (*card.Data, error) { return nil, nil }

func TestResolveRowsStreamsEveryRow(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	rows, _, _ := cardlist.Parse("Lightning Bolt\nCounterspell", "")
	j := ws.Jobs.New()
	svc.resolveRows(context.Background(), j, cardlist.APIResolver(noCards{}), rows)

	events, finished := j.Backlog(0)
	if !finished {
		t.Fatal("job did not finish")
	}
	seen := 0
	for i, e := range events {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
		if e.Row != nil {
			seen++
		}
	}
	last := events[len(events)-1]
	if seen != 2 || !last.Done || last.Err != "" || !strings.Contains(last.Step, "2 of 2") {
		t.Errorf("%d row events, last event = %+v", seen, last)
	}
}

func TestReadFile(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	path := filepath.Join(t.TempDir(), "deck.txt")
	if err := os.WriteFile(path, []byte("4 Lightning Bolt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.ReadFile(path); err != nil || got != "4 Lightning Bolt\n" {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
	if _, err := svc.ReadFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("a missing file read without error")
	}
}
