package main

import (
	"image"
	"sync"
	"time"
)

// jobEvent is one message in a job's progress stream. Step and Frac drive a
// progress bar, and the terminal event sets Done, with ArtMissing on a render
// whose art could not be fetched or Err on a failure. A list resolve carries one
// resolved Row per event and a batch run carries one Card update, so a single
// stream reports on every item in the job
type jobEvent struct {
	Step       string       `json:"step,omitempty"`
	Frac       float64      `json:"frac"`
	Done       bool         `json:"done,omitempty"`
	ArtMissing bool         `json:"artMissing,omitempty"`
	Err        string       `json:"error,omitempty"`
	Row        *resolvedRow `json:"row,omitempty"`
	Card       *runCard     `json:"card,omitempty"`
	Log        string       `json:"log,omitempty"`
}

// job is one render or template-download in flight. It records every event so a
// subscriber that connects a moment late still replays from the start, and wakes
// waiting subscribers as events arrive. A render job also holds its finished
// image and the card name for the download filename
type job struct {
	mu       sync.Mutex
	events   []jobEvent
	finished bool
	waiters  []chan struct{}

	img     image.Image
	name    string
	created time.Time
}

// emit appends an event and wakes any subscribers. A Done event marks the job
// finished, which ends every stream after the backlog drains
func (j *job) emit(e jobEvent) {
	j.mu.Lock()
	j.events = append(j.events, e)
	if e.Done {
		j.finished = true
	}
	for _, w := range j.waiters {
		close(w)
	}
	j.waiters = nil
	j.mu.Unlock()
}

// setResult records a finished render's image and card name, read later by the
// image endpoint
func (j *job) setResult(img image.Image, name string) {
	j.mu.Lock()
	j.img = img
	j.name = name
	j.mu.Unlock()
}

// result returns the finished image and card name, or a nil image when the job
// has none yet
func (j *job) result() (image.Image, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.img, j.name
}

// stream returns the events after index from, whether the job has finished, and
// a channel that closes when more events arrive. The caller writes the returned
// events, then either returns (finished) or waits on the channel before calling
// again. This replays the backlog to a late subscriber and blocks only between
// events
func (j *job) stream(from int) (events []jobEvent, finished bool, wait chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	events = append([]jobEvent(nil), j.events[from:]...)
	if j.finished {
		return events, true, nil
	}
	wait = make(chan struct{})
	j.waiters = append(j.waiters, wait)
	return events, false, wait
}
