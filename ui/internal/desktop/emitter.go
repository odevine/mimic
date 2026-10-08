package desktop

import (
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/jobs"
)

// jobEventName is the one window event every job's progress travels on
const jobEventName = "job"

// flushEvery is the shortest gap between two sends for one job. A batch at full
// speed reports dozens of cards a second, and the page repaints on each send, so
// events inside the gap go out together in the next one
const flushEvery = 50 * time.Millisecond

// JobEvents is the payload of a job window event: the job and the events it
// emitted since the last send, in order. The events carry their own sequence
// numbers, so the page drops any it has already applied
type JobEvents struct {
	ID     string       `json:"id"`
	Events []jobs.Event `json:"events"`
}

// emitter sends job events to the page, holding back the frequent ones. It
// sends at once when a job has been quiet, so a first progress step is not late,
// and always sends at once for a job's last event, with everything held for it
type emitter struct {
	send func(name string, data any)
	now  func() time.Time

	mu   sync.Mutex
	jobs map[string]*jobQueue
}

// jobQueue holds a job's unsent events and when it last sent
type jobQueue struct {
	pending []jobs.Event
	last    time.Time
	timer   *time.Timer
}

func newEmitter(send func(name string, data any)) *emitter {
	return &emitter{send: send, now: time.Now, jobs: make(map[string]*jobQueue)}
}

// Emit queues an event for the page. It implements jobs.Emitter
func (e *emitter) Emit(jobID string, ev jobs.Event) {
	e.mu.Lock()
	q := e.jobs[jobID]
	if q == nil {
		q = &jobQueue{}
		e.jobs[jobID] = q
	}
	q.pending = append(q.pending, ev)

	switch wait := flushEvery - e.now().Sub(q.last); {
	case ev.Done || wait <= 0:
		e.flushLocked(jobID, q)
		if ev.Done {
			delete(e.jobs, jobID)
		}
	case q.timer == nil:
		q.timer = time.AfterFunc(wait, func() { e.flush(jobID) })
	}
	e.mu.Unlock()
}

// flush sends a job's held events
func (e *emitter) flush(jobID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if q := e.jobs[jobID]; q != nil {
		e.flushLocked(jobID, q)
	}
}

func (e *emitter) flushLocked(jobID string, q *jobQueue) {
	if q.timer != nil {
		q.timer.Stop()
		q.timer = nil
	}
	if len(q.pending) == 0 {
		return
	}
	events := q.pending
	q.pending = nil
	q.last = e.now()
	e.send(jobEventName, JobEvents{ID: jobID, Events: events})
}
