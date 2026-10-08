package desktop

import (
	"sync"
	"testing"
	"time"

	"github.com/odevine/mimic/ui/internal/jobs"
)

// sent collects what the emitter sends
type sent struct {
	mu   sync.Mutex
	got  []JobEvents
	name string
}

func (s *sent) send(name string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
	s.got = append(s.got, data.(JobEvents))
}

func (s *sent) eventName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.name
}

func (s *sent) sends() []JobEvents {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]JobEvents(nil), s.got...)
}

func ev(seq int) jobs.Event { return jobs.Event{Seq: seq, Step: "x"} }

func TestEmitterSendsTheFirstEventAtOnceAndBatchesTheRest(t *testing.T) {
	s := &sent{}
	e := newEmitter(s.send)
	for i := 1; i <= 5; i++ {
		e.Emit("1", ev(i))
	}

	got := s.sends()
	if len(got) != 1 || len(got[0].Events) != 1 || got[0].ID != "1" || s.eventName() != "job" {
		t.Fatalf("right after the burst: %+v, want only the first event out", got)
	}

	time.Sleep(flushEvery + 40*time.Millisecond)
	got = s.sends()
	if len(got) != 2 || len(got[1].Events) != 4 {
		t.Fatalf("after the gap: %+v, want the other four in one send", got)
	}
	for i, e := range got[1].Events {
		if e.Seq != i+2 {
			t.Errorf("held event %d has seq %d, want them in order", i, e.Seq)
		}
	}
}

func TestEmitterFlushesEverythingHeldWhenAJobEnds(t *testing.T) {
	s := &sent{}
	e := newEmitter(s.send)
	e.Emit("1", ev(1))
	e.Emit("1", ev(2))
	e.Emit("1", ev(3))
	e.Emit("1", jobs.Event{Seq: 4, Done: true})

	got := s.sends()
	if len(got) != 2 || len(got[1].Events) != 3 || !got[1].Events[2].Done {
		t.Fatalf("sends = %+v, want the held events and the done event out together", got)
	}
	time.Sleep(flushEvery + 40*time.Millisecond)
	if n := len(s.sends()); n != 2 {
		t.Errorf("%d sends after the job ended, want no more", n)
	}
}

func TestEmitterKeepsJobsApart(t *testing.T) {
	s := &sent{}
	e := newEmitter(s.send)
	e.Emit("1", ev(1))
	e.Emit("2", ev(1))
	got := s.sends()
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Errorf("sends = %+v, want each job's first event out at once", got)
	}
}
