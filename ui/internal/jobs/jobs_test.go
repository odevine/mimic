package jobs

import (
	"sync"
	"testing"
)

// recorder remembers every event the registry forwards
type recorder struct {
	mu  sync.Mutex
	got []string
	seq []int
}

func (r *recorder) Emit(id string, e Event) {
	r.mu.Lock()
	r.got = append(r.got, id)
	r.seq = append(r.seq, e.Seq)
	r.mu.Unlock()
}

func TestEventsAreNumberedAndReplayed(t *testing.T) {
	reg := NewRegistry()
	rec := &recorder{}
	reg.SetEmitter(rec)
	j := reg.New()
	for range 3 {
		j.Emit(Event{Step: "x"})
	}
	j.Emit(Event{Done: true})

	events, finished := j.Backlog(0)
	if !finished || len(events) != 4 {
		t.Fatalf("backlog = %d events, finished %v", len(events), finished)
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
	}
	// A watcher that applied the first two gets the rest
	if rest, _ := j.Backlog(2); len(rest) != 2 || rest[0].Seq != 3 {
		t.Errorf("backlog after 2 = %+v", rest)
	}
	if len(rec.got) != 4 || rec.got[0] != j.ID() || rec.seq[3] != 4 {
		t.Errorf("emitter saw %v %v", rec.got, rec.seq)
	}
}

func TestBacklogService(t *testing.T) {
	reg := NewRegistry()
	svc := NewService(reg)
	j := reg.New()
	j.Emit(Event{Step: "a"})
	got, err := svc.Backlog(j.ID(), 0)
	if err != nil || len(got.Events) != 1 || got.Finished {
		t.Errorf("backlog = %+v, %v", got, err)
	}
	if _, err := svc.Backlog("nope", 0); err == nil {
		t.Error("an unknown job had a backlog")
	}
}

func TestNamedJobsOutliveTheReaper(t *testing.T) {
	reg := NewRegistry()
	j := reg.NewNamed("run-1")
	j.Emit(Event{Done: true})
	j.created = j.created.Add(-2 * ttl)
	reg.New()
	if _, ok := reg.Lookup("run-1"); !ok {
		t.Error("a named job was reaped")
	}
	reg.Forget("run-1")
	if _, ok := reg.Lookup("run-1"); ok {
		t.Error("a forgotten job is still there")
	}
}

func TestFinishedJobsAreReaped(t *testing.T) {
	reg := NewRegistry()
	old := reg.New()
	old.Emit(Event{Done: true})
	old.created = old.created.Add(-2 * ttl)
	reg.New()
	if _, ok := reg.Lookup(old.ID()); ok {
		t.Error("an expired finished job was kept")
	}
}

// Events from goroutines emitting at once must reach the emitter in the order of
// their numbers, since the page drops an event numbered below one it has applied
func TestConcurrentEventsReachTheEmitterInOrder(t *testing.T) {
	reg := NewRegistry()
	rec := &recorder{}
	reg.SetEmitter(rec)
	j := reg.New()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				j.Emit(Event{Step: "row"})
			}
		}()
	}
	wg.Wait()

	if len(rec.seq) != 4000 {
		t.Fatalf("the emitter heard %d events, want 4000", len(rec.seq))
	}
	for i := 1; i < len(rec.seq); i++ {
		if rec.seq[i] != rec.seq[i-1]+1 {
			t.Fatalf("event %d arrived after %d: the emitter heard them out of order", rec.seq[i], rec.seq[i-1])
		}
	}
}
