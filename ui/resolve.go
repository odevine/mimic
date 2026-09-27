package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/odevine/mimic/ui/internal/cardlist"
)

// Card data sources, as the settings store them
const (
	cardDataAPI   = "api"
	cardDataLocal = "local"
)

// resolveBody is the POST /api/resolve payload. An empty format sniffs one
type resolveBody struct {
	Text   string `json:"text"`
	Format string `json:"format"`
}

// handleResolve parses a list and starts resolving its rows. It answers at once
// with the parsed rows, so the table can draw every line before any lookup
// returns, and the job streams one event per resolved row. Starting a resolve
// abandons any earlier one still running
func (s *server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var body resolveBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad resolve request: "+err.Error(), http.StatusBadRequest)
		return
	}
	rows, format, err := cardlist.Parse(body.Text, body.Format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rows == nil {
		rows = []cardlist.Row{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.resolveMu.Lock()
	if s.resolveCancel != nil {
		s.resolveCancel()
	}
	s.resolveCancel = cancel
	s.resolveMu.Unlock()

	id, j := s.newJob()
	go func() {
		defer cancel()
		resolveRows(ctx, j, s.resolver(), &s.resolved, rows)
	}()
	writeJSON(w, map[string]any{"jobId": id, "format": format, "rows": rows})
}

// resolveRows looks every row up and emits each result as it lands, in
// completion order with its index, then a final done event. A cancelled resolve
// ends without a done event's error, since the page has already moved on
func resolveRows(ctx context.Context, j *job, rv cardlist.Resolver, cache *cardlist.Cache, rows []cardlist.Row) {
	err := cardlist.ResolveAll(ctx, rv, cache, rows, func(n int, res cardlist.Resolved) {
		j.emit(jobEvent{
			Step: fmt.Sprintf("Resolved %d of %d", n, len(rows)),
			Frac: float64(n) / float64(len(rows)),
			Row:  &res,
		})
	})
	if err != nil {
		j.emit(jobEvent{Done: true, Err: "resolve superseded"})
		return
	}
	j.emit(jobEvent{Done: true, Frac: 1, Step: fmt.Sprintf("Resolved %d of %d", len(rows), len(rows))})
}

// resolver picks the lookup source for a new resolve. The local copy is used
// when it is chosen in settings and loaded, and the API otherwise
func (s *server) resolver() cardlist.Resolver {
	if s.useLocalCards() {
		return cardlist.LocalResolver(s.cards, s.pipe.client)
	}
	return cardlist.APIResolver(s.pipe.client)
}

// useLocalCards reports whether lookups should read the local copy
func (s *server) useLocalCards() bool {
	return s.cards != nil && s.prefs.Settings().CardData == cardDataLocal && s.cards.Ready()
}
