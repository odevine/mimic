package main

import "strings"

// maxRecent bounds how many past queries the search box remembers
const maxRecent = 10

// recents is a most-recent-first, de-duplicated, bounded list of search
// queries. Matching is case-insensitive on the trimmed text but the stored
// entry keeps the casing of the newest use
type recents struct {
	items []string
}

// newRecents builds a list from stored entries, applying the same trimming,
// de-duplication, and cap that add enforces so a hand-edited store stays valid
func newRecents(stored []string) *recents {
	r := &recents{}
	// Replay oldest-first so the newest stored entry ends up at the front
	for i := len(stored) - 1; i >= 0; i-- {
		r.add(stored[i])
	}
	return r
}

// add records a query at the front, dropping any earlier case-insensitive
// duplicate and trimming the list to maxRecent. A blank query is ignored
func (r *recents) add(query string) {
	q := strings.TrimSpace(query)
	if q == "" {
		return
	}
	kept := make([]string, 0, len(r.items)+1)
	kept = append(kept, q)
	for _, prev := range r.items {
		if strings.EqualFold(prev, q) {
			continue
		}
		kept = append(kept, prev)
		if len(kept) == maxRecent {
			break
		}
	}
	r.items = kept
}

// list returns the queries newest-first. The caller must not mutate the result
func (r *recents) list() []string { return r.items }
