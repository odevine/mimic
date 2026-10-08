package render

import (
	"context"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func TestCancelStopsJob(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	j := ws.Jobs.New()
	ctx, cancel := context.WithCancel(context.Background())
	j.SetCancel(cancel)

	if err := svc.Cancel(j.ID()); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("job context still live after cancel")
	}
	if err := svc.Cancel("nope"); apierr.KindOf(err) != apierr.NotFound {
		t.Fatalf("unknown job: %v, want not found", err)
	}
}

func TestFilename(t *testing.T) {
	for in, want := range map[string]string{"": "card.png", "Sol Ring": "Sol Ring.png", "Fire // Ice": "Fire -- Ice.png"} {
		if got := Filename(in); got != want {
			t.Errorf("Filename(%q) = %q, want %q", in, got, want)
		}
	}
}
