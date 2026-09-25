package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/odevine/mimic/engine/card"
)

// A local copy of Scryfall's bulk data, so resolving a list reads from disk
// instead of queueing behind the API's rate limit. The store keeps one trimmed
// record per printing in cards.jsonl and an index of names, printings and each
// card's default printing in index.gob, which loads into memory at startup

// localCardFormat versions the files the store writes. A copy in an older
// format is ignored, and the settings panel offers a fresh download
const localCardFormat = 1

// The store's states as the settings panel shows them
const (
	cardDataNone    = "none"
	cardDataLoading = "loading"
	cardDataReady   = "ready"
	cardDataError   = "error"
)

// Entry flags
const (
	flagPromo uint8 = 1 << iota
	flagDigital
	flagPaper
	flagDefault
)

// localMeta describes an installed copy
type localMeta struct {
	Format    int       `json:"format"`
	UpdatedAt time.Time `json:"updatedAt"`
	BuiltAt   time.Time `json:"builtAt"`
	Printings int       `json:"printings"`
	Bytes     int64     `json:"bytes"`
}

// localEntry is one printing in the index. Off and Len locate its card record
// in cards.jsonl
type localEntry struct {
	Off      int64
	Len      int32
	Name     string
	Faces    []string
	Set      string
	Number   string
	Oracle   string
	Released string
	Rank     int32
	Flags    uint8
}

// localIndex is what index.gob holds, plus the lookup maps built from it
type localIndex struct {
	Entries []localEntry

	byName      map[string][]int32
	bySetNumber map[string]int32
	byOracle    map[string][]int32
	defaults    map[string]int32
	names       []nameRef
}

// nameRef is one searchable name, a card's full name or one of its faces
type nameRef struct {
	norm   string
	oracle string
	rank   int32
}

// localStore is the installed copy and its state. Reads take the read lock for
// the length of a record read, so installing a new copy never closes a file
// under a reader
type localStore struct {
	dir string

	mu    sync.RWMutex
	state string
	err   string
	meta  localMeta
	idx   *localIndex
	file  *os.File

	// jobID is the download in progress, "" when there is none
	jobID string
}

func newLocalStore(dir string) *localStore {
	return &localStore{dir: dir, state: cardDataNone}
}

// localCardDir is where the store keeps its files, beside the template cache
func localCardDir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "scryfall"), nil
}

func (s *localStore) current() string { return filepath.Join(s.dir, "current") }

// load opens the installed copy, if there is one, and builds its lookup maps. It
// runs in the background at startup, since reading the index takes a moment
func (s *localStore) load() {
	s.mu.Lock()
	if s.dir == "" {
		s.mu.Unlock()
		return
	}
	s.state = cardDataLoading
	s.mu.Unlock()

	meta, idx, f, err := openLocalCards(s.current())
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.state = cardDataNone
	case err != nil:
		s.state, s.err = cardDataError, err.Error()
	default:
		if s.file != nil {
			s.file.Close()
		}
		s.state, s.err, s.meta, s.idx, s.file = cardDataReady, "", meta, idx, f
	}
}

// openLocalCards reads an installed copy from dir
func openLocalCards(dir string) (localMeta, *localIndex, *os.File, error) {
	var meta localMeta
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return meta, nil, nil, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, nil, nil, fmt.Errorf("reading card data: %w", err)
	}
	if meta.Format != localCardFormat {
		return meta, nil, nil, fmt.Errorf("the local card data is from an older version of mimic, download it again")
	}
	ix, err := os.Open(filepath.Join(dir, "index.gob"))
	if err != nil {
		return meta, nil, nil, err
	}
	defer ix.Close()
	var idx localIndex
	if err := gob.NewDecoder(bufio.NewReader(ix)).Decode(&idx); err != nil {
		return meta, nil, nil, fmt.Errorf("reading card index: %w", err)
	}
	idx.build()
	f, err := os.Open(filepath.Join(dir, "cards.jsonl"))
	if err != nil {
		return meta, nil, nil, err
	}
	return meta, &idx, f, nil
}

// build derives the lookup maps from the entries
func (x *localIndex) build() {
	x.byName = make(map[string][]int32)
	x.bySetNumber = make(map[string]int32)
	x.byOracle = make(map[string][]int32)
	x.defaults = make(map[string]int32)
	seen := make(map[string]bool)
	for i, e := range x.Entries {
		n := int32(i)
		names := append([]string{e.Name}, e.Faces...)
		for _, name := range names {
			k := normName(name)
			if k == "" {
				continue
			}
			x.byName[k] = append(x.byName[k], n)
			if key := e.Oracle + "\x00" + k; !seen[key] {
				seen[key] = true
				x.names = append(x.names, nameRef{norm: k, oracle: e.Oracle, rank: e.Rank})
			}
		}
		x.bySetNumber[e.Set+"\x00"+strings.ToLower(e.Number)] = n
		x.byOracle[e.Oracle] = append(x.byOracle[e.Oracle], n)
		if e.Flags&flagDefault != 0 {
			x.defaults[e.Oracle] = n
		}
	}
}

// localStatus is what GET /api/carddata reports about the installed copy
type localStatus struct {
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	Printings int       `json:"printings,omitempty"`
	Bytes     int64     `json:"bytes,omitempty"`
	JobID     string    `json:"jobId,omitempty"`
}

func (s *localStore) status() localStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := localStatus{State: s.state, Error: s.err, JobID: s.jobID}
	if s.state == cardDataReady {
		st.UpdatedAt, st.Printings, st.Bytes = s.meta.UpdatedAt, s.meta.Printings, s.meta.Bytes
	}
	return st
}

func (s *localStore) ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == cardDataReady
}

// remove deletes the installed copy
func (s *localStore) remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobID != "" {
		return errors.New("a card data download is in progress")
	}
	if s.file != nil {
		s.file.Close()
	}
	s.state, s.err, s.meta, s.idx, s.file = cardDataNone, "", localMeta{}, nil, nil
	return os.RemoveAll(s.current())
}

// record reads one printing's card by its position in x. A copy installed since
// x was read has different positions, so reading from it is refused
func (s *localStore) record(x *localIndex, i int32) (*card.Data, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.idx == nil || s.idx != x {
		return nil, errors.New("the local card data changed, try again")
	}
	e := x.Entries[i]
	buf := make([]byte, e.Len)
	if _, err := s.file.ReadAt(buf, e.Off); err != nil {
		return nil, fmt.Errorf("reading local card data: %w", err)
	}
	var d card.Data
	if err := json.Unmarshal(buf, &d); err != nil {
		return nil, fmt.Errorf("reading local card data: %w", err)
	}
	return &d, nil
}

// index returns the loaded index, or nil before one is loaded
func (s *localStore) index() *localIndex {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idx
}

// oracleFor picks the card a name means. An exact name can match more than one
// card when a face shares it, as Swords to Plowshares does with a double-faced
// card whose back face carries the same name, so a card whose full name or
// front face is the name wins
func (x *localIndex) oracleFor(name string) (string, bool) {
	k := normName(name)
	hits := x.byName[k]
	if len(hits) == 0 {
		return "", false
	}
	best, score := "", -1
	for _, i := range hits {
		e := x.Entries[i]
		s := 0
		switch {
		case normName(e.Name) == k:
			s = 2
		case len(e.Faces) > 0 && normName(e.Faces[0]) == k:
			s = 1
		}
		if s > score {
			best, score = e.Oracle, s
		}
	}
	return best, true
}

// defaultPrinting is the printing a name with no set given resolves to:
// Scryfall's own pick for the card when the copy has it, otherwise the newest
// paper printing that is not a promo
func (x *localIndex) defaultPrinting(oracle string) (int32, bool) {
	if i, ok := x.defaults[oracle]; ok {
		return i, true
	}
	return x.pick(x.byOracle[oracle])
}

// pick chooses the most ordinary printing among candidates: paper, not a promo,
// not digital, and newest
func (x *localIndex) pick(cands []int32) (int32, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	score := func(e localEntry) int {
		s := 0
		if e.Flags&flagPaper != 0 {
			s += 4
		}
		if e.Flags&flagPromo == 0 {
			s += 2
		}
		if e.Flags&flagDigital == 0 {
			s++
		}
		return s
	}
	best := cands[0]
	for _, i := range cands[1:] {
		a, b := x.Entries[i], x.Entries[best]
		if sa, sb := score(a), score(b); sa > sb || sa == sb && a.Released > b.Released {
			best = i
		}
	}
	return best, true
}

// exact is the card with this exact name at its default printing, or nil
func (s *localStore) exact(name string) (*card.Data, error) {
	x := s.index()
	if x == nil {
		return nil, nil
	}
	oracle, ok := x.oracleFor(name)
	if !ok {
		return nil, nil
	}
	i, _ := x.defaultPrinting(oracle)
	return s.record(x, i)
}

// printing is the named card's printing in a set, and at a collector number
// when one is given, or nil when the copy has no such printing
func (s *localStore) printing(name, set, number string) (*card.Data, error) {
	x := s.index()
	if x == nil {
		return nil, nil
	}
	oracle, ok := x.oracleFor(name)
	if !ok {
		return nil, nil
	}
	set = strings.ToLower(set)
	if number != "" {
		i, ok := x.bySetNumber[set+"\x00"+strings.ToLower(number)]
		if !ok || x.Entries[i].Oracle != oracle {
			return nil, nil
		}
		return s.record(x, i)
	}
	var inSet []int32
	for _, i := range x.byOracle[oracle] {
		if x.Entries[i].Set == set {
			inSet = append(inSet, i)
		}
	}
	i, ok := x.pick(inSet)
	if !ok {
		return nil, nil
	}
	return s.record(x, i)
}

// printings lists every printing of the named card, newest first
func (s *localStore) printings(name string) ([]*card.Data, error) {
	x := s.index()
	if x == nil {
		return nil, nil
	}
	oracle, ok := x.oracleFor(name)
	if !ok {
		return nil, nil
	}
	ids := slices.Clone(x.byOracle[oracle])
	slices.SortStableFunc(ids, func(a, b int32) int { return strings.Compare(x.Entries[b].Released, x.Entries[a].Released) })
	out := make([]*card.Data, 0, len(ids))
	for _, i := range ids {
		d, err := s.record(x, i)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// similar lists up to n cards whose names contain every word of name, most
// played first, each at its default printing
func (s *localStore) similar(name string, n int) ([]*card.Data, error) {
	x := s.index()
	if x == nil {
		return nil, nil
	}
	words := strings.Fields(normName(name))
	if len(words) == 0 {
		return nil, nil
	}
	var hits []nameRef
	seen := make(map[string]bool)
	for _, r := range x.names {
		if seen[r.oracle] {
			continue
		}
		all := true
		for _, w := range words {
			if !strings.Contains(r.norm, w) {
				all = false
				break
			}
		}
		if all {
			seen[r.oracle] = true
			hits = append(hits, r)
		}
	}
	// Unranked cards sort after every ranked one
	rank := func(r nameRef) int32 {
		if r.rank <= 0 {
			return 1 << 30
		}
		return r.rank
	}
	slices.SortStableFunc(hits, func(a, b nameRef) int { return int(rank(a) - rank(b)) })
	var out []*card.Data
	for _, r := range hits[:min(len(hits), n)] {
		i, ok := x.defaultPrinting(r.oracle)
		if !ok {
			continue
		}
		d, err := s.record(x, i)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// fuzzy is the card whose name is closest to a mistyped one, allowing about one
// edit for every four letters, or nil when nothing is that close
func (s *localStore) fuzzy(name string) (*card.Data, error) {
	x := s.index()
	if x == nil {
		return nil, nil
	}
	k := normName(name)
	limit := max(1, len([]rune(k))/4)
	best, bestD := "", limit+1
	for _, r := range x.names {
		if d := editDistance(k, r.norm, bestD); d < bestD || d == bestD && best != "" && r.rank > 0 && r.rank < x.rankOf(best) {
			best, bestD = r.oracle, d
		}
	}
	if best == "" || bestD > limit {
		return nil, nil
	}
	i, ok := x.defaultPrinting(best)
	if !ok {
		return nil, nil
	}
	return s.record(x, i)
}

func (x *localIndex) rankOf(oracle string) int32 {
	for _, i := range x.byOracle[oracle] {
		if r := x.Entries[i].Rank; r > 0 {
			return r
		}
	}
	return 1 << 30
}

// editDistance is the Levenshtein distance between a and b, giving up with
// limit+1 as soon as the distance must exceed limit
func editDistance(a, b string, limit int) int {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > limit || -d > limit {
		return limit + 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > limit {
			return limit + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// foldAccents maps the accented letters that appear in card names to the plain
// letters a user types, as in Lim-Dûl's Vault
var foldAccents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "æ", "ae",
	"’", "'", "‘", "'",
)

// normName is the form names are compared in: lowercase, accents folded, and
// runs of spaces collapsed
func normName(s string) string {
	s = foldAccents.Replace(strings.ToLower(s))
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// bulkCard is the part of a bulk data line the index needs, read beside the
// engine's own mapping of the same line
type bulkCard struct {
	ID              string   `json:"id"`
	OracleID        string   `json:"oracle_id"`
	Name            string   `json:"name"`
	Layout          string   `json:"layout"`
	Set             string   `json:"set"`
	CollectorNumber string   `json:"collector_number"`
	ReleasedAt      string   `json:"released_at"`
	Promo           bool     `json:"promo"`
	Digital         bool     `json:"digital"`
	Games           []string `json:"games"`
	EdhrecRank      int32    `json:"edhrec_rank"`
	CardFaces       []struct {
		Name     string `json:"name"`
		OracleID string `json:"oracle_id"`
	} `json:"card_faces"`
}

// oracle is the card's Oracle id. A reversible card carries one on each face
// rather than at the top
func (b bulkCard) oracle() string {
	if b.OracleID == "" && len(b.CardFaces) > 0 {
		return b.CardFaces[0].OracleID
	}
	return b.OracleID
}

// buildLocalCards writes a store into dir from Scryfall's oracle_cards and
// default_cards bulk files, both already decompressed. oracle only marks which
// printing Scryfall picks for each card. progress, when set, is told how many
// printings have been read
func buildLocalCards(ctx context.Context, dir string, oracle, cards io.Reader, updatedAt time.Time, progress func(printings int)) (localMeta, error) {
	defaults := make(map[string]bool)
	err := eachLine(ctx, oracle, func(line []byte) error {
		var b bulkCard
		if err := json.Unmarshal(line, &b); err != nil {
			return err
		}
		defaults[b.ID] = true
		return nil
	})
	if err != nil {
		return localMeta{}, fmt.Errorf("reading oracle cards: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return localMeta{}, err
	}
	f, err := os.Create(filepath.Join(dir, "cards.jsonl"))
	if err != nil {
		return localMeta{}, err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	var idx localIndex
	var off int64
	err = eachLine(ctx, cards, func(line []byte) error {
		var b bulkCard
		if err := json.Unmarshal(line, &b); err != nil {
			return err
		}
		// Art series cards are the art cards packed beside a set, which share a
		// real card's name without being one
		if b.Layout == "art_series" || b.ID == "" {
			return nil
		}
		d, err := card.FromScryfallJSON(line)
		if err != nil {
			return err
		}
		d.ArtworkURL = card.ArtCropURL(b.ID)
		rec, err := json.Marshal(d)
		if err != nil {
			return err
		}
		rec = append(rec, '\n')
		if _, err := w.Write(rec); err != nil {
			return err
		}
		e := localEntry{
			Off:      off,
			Len:      int32(len(rec) - 1),
			Name:     b.Name,
			Set:      strings.ToLower(b.Set),
			Number:   b.CollectorNumber,
			Oracle:   b.oracle(),
			Released: b.ReleasedAt,
			Rank:     b.EdhrecRank,
		}
		for _, fc := range b.CardFaces {
			e.Faces = append(e.Faces, fc.Name)
		}
		if b.Promo {
			e.Flags |= flagPromo
		}
		if b.Digital {
			e.Flags |= flagDigital
		}
		if slices.Contains(b.Games, "paper") {
			e.Flags |= flagPaper
		}
		if defaults[b.ID] {
			e.Flags |= flagDefault
		}
		idx.Entries = append(idx.Entries, e)
		off += int64(len(rec))
		if progress != nil && len(idx.Entries)%2000 == 0 {
			progress(len(idx.Entries))
		}
		return nil
	})
	if err != nil {
		return localMeta{}, fmt.Errorf("reading default cards: %w", err)
	}
	if err := w.Flush(); err != nil {
		return localMeta{}, err
	}
	if len(idx.Entries) == 0 {
		return localMeta{}, errors.New("the card data file held no cards")
	}

	ix, err := os.Create(filepath.Join(dir, "index.gob"))
	if err != nil {
		return localMeta{}, err
	}
	bw := bufio.NewWriter(ix)
	if err := gob.NewEncoder(bw).Encode(&idx); err != nil {
		ix.Close()
		return localMeta{}, err
	}
	if err := bw.Flush(); err != nil {
		ix.Close()
		return localMeta{}, err
	}
	if err := ix.Close(); err != nil {
		return localMeta{}, err
	}

	meta := localMeta{Format: localCardFormat, UpdatedAt: updatedAt, BuiltAt: time.Now(), Printings: len(idx.Entries), Bytes: off}
	raw, _ := json.MarshalIndent(meta, "", "  ")
	return meta, os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o644)
}

// maxBulkLine bounds one line of a bulk file. Real card objects are a few
// kilobytes, and the largest are under a hundred
const maxBulkLine = 4 << 20

// eachLine calls fn with each non-empty line of r, checking ctx as it goes
func eachLine(ctx context.Context, r io.Reader, fn func([]byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxBulkLine)
	for n := 0; sc.Scan(); n++ {
		if n%1000 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return sc.Err()
}

// cardDir is the store's folder, or "" when there is no config directory, in
// which case local card data is simply unavailable
func cardDir() string {
	dir, err := localCardDir()
	if err != nil {
		return ""
	}
	return dir
}
