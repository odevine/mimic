package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/odevine/mimic/engine/card"
)

// The states a resolved row lands in. Error is a lookup that failed for a reason
// other than the card not existing, such as the network, and is worth retrying
const (
	rowMatched   = "matched"
	rowAmbiguous = "ambiguous"
	rowNotFound  = "notFound"
	rowCustom    = "custom"
	rowError     = "error"
)

// queryCap bounds how many cards one ? line expands to, so a broad query cannot
// turn a short list into hundreds of renders without the user noticing
const queryCap = 100

// maxCandidates bounds the choices an ambiguous row offers
const maxCandidates = 8

// resolveWorkers is how many rows look up at once. Scryfall calls are paced by
// the shared transport regardless, so this only overlaps each call's latency
// with the next one's wait
const resolveWorkers = 3

// resolveCacheMax bounds the session cache of lookups. It is cleared whole when
// full, which is simpler than an eviction order and rare in practice
const resolveCacheMax = 4000

// cardSource is the part of the Scryfall client resolving needs, so a test can
// answer lookups without a network
type cardSource interface {
	Search(ctx context.Context, query string) ([]*card.Data, error)
	FetchByName(ctx context.Context, name string) (*card.Data, error)
}

// resolvedRow is the outcome of one list row. Card is the match, or the
// preselected choice of an ambiguous row. Cards is a query line's expansion
type resolvedRow struct {
	Index      int          `json:"index"`
	Status     string       `json:"status"`
	Card       *card.Data   `json:"card,omitempty"`
	Candidates []*card.Data `json:"candidates,omitempty"`
	Cards      []*card.Data `json:"cards,omitempty"`
	Note       string       `json:"note,omitempty"`
}

// resolveCache remembers lookups for the session, keyed on what was asked
type resolveCache struct {
	mu sync.Mutex
	m  map[string]resolvedRow
}

func (c *resolveCache) get(key string) (resolvedRow, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.m[key]
	return r, ok
}

func (c *resolveCache) put(key string, r resolvedRow) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= resolveCacheMax {
		c.m = make(map[string]resolvedRow)
	}
	c.m[key] = r
}

// cacheKey identifies a lookup. Fields are left out, since a CSV row's overrides
// are applied by the page on top of whatever the name resolves to
func (row listRow) cacheKey() string {
	return strings.ToLower(strings.Join([]string{row.Query, row.Name, row.Set, row.Number}, "\x00"))
}

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
	rows, format, err := parseList(body.Text, body.Format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rows == nil {
		rows = []listRow{}
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
		resolveRows(ctx, j, s.pipe.client, &s.resolved, rows)
	}()
	writeJSON(w, map[string]any{"jobId": id, "format": format, "rows": rows})
}

// resolveRows looks every row up and emits each result as it lands, in
// completion order with its index, then a final done event. A cancelled resolve
// ends without a done event's error, since the page has already moved on
func resolveRows(ctx context.Context, j *job, src cardSource, cache *resolveCache, rows []listRow) {
	next := make(chan int)
	var mu sync.Mutex
	done := 0
	var wg sync.WaitGroup
	for range min(resolveWorkers, max(len(rows), 1)) {
		wg.Go(func() {
			for i := range next {
				res := resolveRow(ctx, src, cache, rows[i])
				res.Index = i
				mu.Lock()
				done++
				n := done
				mu.Unlock()
				if ctx.Err() != nil {
					continue
				}
				j.emit(jobEvent{
					Step: fmt.Sprintf("Resolved %d of %d", n, len(rows)),
					Frac: float64(n) / float64(len(rows)),
					Row:  &res,
				})
			}
		})
	}
	for i := range rows {
		select {
		case next <- i:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(next)
	wg.Wait()
	if ctx.Err() != nil {
		j.emit(jobEvent{Done: true, Err: "resolve superseded"})
		return
	}
	j.emit(jobEvent{Done: true, Frac: 1, Step: fmt.Sprintf("Resolved %d of %d", len(rows), len(rows))})
}

// resolveRow looks one row up, from the cache when it can. Transient failures
// are not cached, so resolving again retries them
func resolveRow(ctx context.Context, src cardSource, cache *resolveCache, row listRow) resolvedRow {
	if row.Custom {
		return customRow(row)
	}
	key := row.cacheKey()
	if res, ok := cache.get(key); ok {
		return withCustomFallback(res, row)
	}
	// No overall deadline here, since a long list spends most of its time queued
	// behind Scryfall's rate limit. Each request times out on its own network wait
	res := lookup(ctx, src, row)
	if res.Status != rowError {
		cache.put(key, res)
	}
	return withCustomFallback(res, row)
}

// customRow builds a card from a row's own fields with no lookup at all
func customRow(row listRow) resolvedRow {
	return resolvedRow{Status: rowCustom, Card: overlayFields(&card.Data{}, row.Fields)}
}

// withCustomFallback turns a CSV row that names no real card into a custom one
// when it carries a type line, since that is enough to render a card that does
// not exist
func withCustomFallback(res resolvedRow, row listRow) resolvedRow {
	if res.Status == rowNotFound && row.Fields["typeLine"] != "" {
		c := customRow(row)
		c.Note = "No Scryfall card has this name, so it renders from its own fields"
		return c
	}
	return res
}

// lookup resolves a row against Scryfall. A query expands to its results. A
// name with a set and collector number looks up that printing first. An exact
// name match is matched with Scryfall's default printing. Anything else tries a
// fuzzy match and a name search, and lands ambiguous with the choices found or
// not found when there are none
func lookup(ctx context.Context, src cardSource, row listRow) resolvedRow {
	if row.Query != "" {
		cards, err := src.Search(ctx, row.Query)
		if err != nil {
			return resolvedRow{Status: rowError, Note: err.Error()}
		}
		if len(cards) == 0 {
			return resolvedRow{Status: rowNotFound, Note: "The query matched no cards"}
		}
		res := resolvedRow{Status: rowMatched, Cards: cards}
		if len(cards) > queryCap {
			res.Cards = cards[:queryCap]
			res.Note = fmt.Sprintf("Capped at the first %d of %d or more matches", queryCap, len(cards))
		}
		return res
	}

	note := ""
	if row.Set != "" {
		q := fmt.Sprintf("!%q set:%s", row.Name, row.Set)
		if row.Number != "" {
			q += " cn:" + row.Number
		}
		cards, err := src.Search(ctx, q+" unique:prints")
		if err != nil {
			return resolvedRow{Status: rowError, Note: err.Error()}
		}
		if len(cards) > 0 {
			return resolvedRow{Status: rowMatched, Card: exactName(cards, row.Name)}
		}
		note = fmt.Sprintf("No printing %s %s, so this is Scryfall's default", strings.ToUpper(row.Set), row.Number)
	}

	cards, err := src.Search(ctx, fmt.Sprintf("!%q", row.Name))
	if err != nil {
		return resolvedRow{Status: rowError, Note: err.Error()}
	}
	if len(cards) > 0 {
		return resolvedRow{Status: rowMatched, Card: exactName(cards, row.Name), Note: note}
	}

	fuzzy, err := src.FetchByName(ctx, row.Name)
	if err != nil && !isNotFound(err) {
		return resolvedRow{Status: rowError, Note: err.Error()}
	}
	// A name search finds the cards whose names contain every word, most played
	// first, which is what makes a short name like Bolt lead with Lightning Bolt.
	// The fuzzy hit is the answer for a typo, which a name search cannot match
	var candidates []*card.Data
	if similar, err := src.Search(ctx, row.Name+" order:edhrec"); err == nil {
		candidates = similar[:min(len(similar), maxCandidates)]
	}
	if fuzzy != nil && !slices.ContainsFunc(candidates, func(c *card.Data) bool { return c.Name == fuzzy.Name }) {
		if len(candidates) == maxCandidates {
			candidates = candidates[:maxCandidates-1]
		}
		candidates = append(candidates, fuzzy)
	}
	if len(candidates) == 0 {
		return resolvedRow{Status: rowNotFound, Note: "No card matched this name"}
	}
	return resolvedRow{Status: rowAmbiguous, Card: candidates[0], Candidates: candidates}
}

// isNotFound reports whether a card client error is Scryfall answering 404,
// which a fuzzy lookup returns both for no match and for too many. The client
// reports status in its error text rather than as a typed error
func isNotFound(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && strings.Contains(err.Error(), "status 404")
}

// exactName picks the card whose own name is the one asked for. An exact-name
// search also matches a double-faced card with a face of that name, and can
// list it first, so Swords to Plowshares would otherwise find a card whose back
// face shares the name. A full two-face name matches no front face, so it falls
// back to the first result
func exactName(cards []*card.Data, name string) *card.Data {
	for _, c := range cards {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return cards[0]
}
