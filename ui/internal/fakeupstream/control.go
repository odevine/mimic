package fakeupstream

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// controlPrefix starts the paths a test uses to change what the fake answers.
// They answer on any host, since a test calls the fake directly
const controlPrefix = "/__fake/"

// The control paths, each taking PUT to set and DELETE to clear:
//
//	/__fake/release   {"version","notes"}            a ui release on offer
//	/__fake/catalog   {"templates":[{"name","version"}]}  template bundles on offer
//	/__fake/fail      {"host","status"}              a host that answers every request with status
//	/__fake/delay     {"host","ms"}                  a host that waits ms before every answer
//	/__fake/requests  GET lists, DELETE forgets      what the app has asked for
func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, controlPrefix)
	switch {
	case name == "requests" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s.Requests())
	case name == "requests" && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.requests = nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "release" && r.Method == http.MethodPut:
		var rel Release
		if err := json.NewDecoder(r.Body).Decode(&rel); err != nil || rel.Version == "" {
			http.Error(w, "fakeupstream: a release needs a version", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.release = &rel
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "release" && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.release = nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "catalog" && r.Method == http.MethodPut:
		var in struct {
			Templates []struct{ Name, Version string } `json:"templates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var out []bundle
		for _, t := range in.Templates {
			b, err := newBundle(t.Name, t.Version)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			out = append(out, b)
		}
		s.mu.Lock()
		s.catalog = out
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "catalog" && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.catalog = nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "fail" && r.Method == http.MethodPut:
		var in struct {
			Host   string `json:"host"`
			Status int    `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Host == "" || in.Status < 400 {
			http.Error(w, "fakeupstream: a failure needs a host and a status of 400 or more", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		if s.failing == nil {
			s.failing = map[string]int{}
		}
		s.failing[in.Host] = in.Status
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "fail" && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.failing = nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "delay" && r.Method == http.MethodPut:
		var in struct {
			Host string `json:"host"`
			Ms   int    `json:"ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Host == "" || in.Ms <= 0 {
			http.Error(w, "fakeupstream: a delay needs a host and a positive number of milliseconds", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		if s.slow == nil {
			s.slow = map[string]time.Duration{}
		}
		s.slow[in.Host] = time.Duration(in.Ms) * time.Millisecond
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case name == "delay" && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.slow = nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "fakeupstream: no control path "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}
