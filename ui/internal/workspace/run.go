package workspace

import (
	"sync"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/jobs"
)

// RunState is the latest batch run, kept after it finishes until the next
// starts, with the job its events go through
type RunState struct {
	mu  sync.Mutex
	run *batch.Run
	job *jobs.Job
	seq uint64
}

// Current returns the latest run, which may have finished, and its job
func (s *RunState) Current() (*batch.Run, *jobs.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run, s.job
}

// Active reports whether a batch is rendering now
func (s *RunState) Active() bool {
	r, _ := s.Current()
	return r != nil && r.Active()
}

// Begin runs start under the run lock if no run is rendering, handing it the
// next run number and the previous job, and keeps the run and job it returns.
// It reports false without calling start when a run is already in progress.
// Holding the lock across start is what keeps two runs from starting at once
func (s *RunState) Begin(start func(seq uint64, prev *jobs.Job) (*batch.Run, *jobs.Job, error)) (ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil && s.run.Active() {
		return false, nil
	}
	s.seq++
	run, job, err := start(s.seq, s.job)
	if err != nil {
		return true, err
	}
	s.run, s.job = run, job
	return true, nil
}

// Set replaces the latest run, for a test that needs one in place
func (s *RunState) Set(run *batch.Run, job *jobs.Job) {
	s.mu.Lock()
	s.run, s.job = run, job
	s.mu.Unlock()
}
