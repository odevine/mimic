// Package fakeupstream answers the requests a test build sends to Scryfall,
// GitHub and the template catalog. A test build redirects every outbound
// request to one address with its original Host header, and this handler tells
// the hosts apart by that header. It serves the card objects in a folder, so a
// test runs without the network and sees the same cards each time
package fakeupstream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Hosts the handler answers for
const (
	ScryfallAPI    = "api.scryfall.com"
	ScryfallImages = "cards.scryfall.io"
	GitHubAPI      = "api.github.com"
	GitHubRaw      = "raw.githubusercontent.com"
)

// Server is the handler and the record of what it was asked
type Server struct {
	cards []card

	mu       sync.Mutex
	requests []string
}

// card is one Scryfall card object and the fields a search reads from it
type card struct {
	raw      json.RawMessage
	name     string
	typeLine string
	set      string
}

// New reads every .json file in dir as a Scryfall card object
func New(dir fs.FS) (*Server, error) {
	files, err := fs.Glob(dir, "*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	s := &Server{}
	for _, f := range files {
		raw, err := fs.ReadFile(dir, f)
		if err != nil {
			return nil, err
		}
		var c struct {
			Name     string `json:"name"`
			TypeLine string `json:"type_line"`
			Set      string `json:"set"`
		}
		if err := json.Unmarshal(raw, &c); err != nil || c.Name == "" {
			return nil, fmt.Errorf("fakeupstream: %s is not a card object: %v", f, err)
		}
		s.cards = append(s.cards, card{raw: raw, name: c.Name, typeLine: c.TypeLine, set: c.Set})
	}
	return s, nil
}

// Requests lists every request seen, as "host path", oldest first
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	s.mu.Lock()
	s.requests = append(s.requests, host+" "+r.URL.Path)
	s.mu.Unlock()

	switch host {
	case ScryfallAPI:
		s.scryfall(w, r)
	case ScryfallImages:
		art(w, r)
	case GitHubAPI:
		// No releases, so the updater reports the app as current
		writeJSON(w, http.StatusOK, []any{})
	case GitHubRaw:
		if strings.HasSuffix(r.URL.Path, "/index.json") {
			writeJSON(w, http.StatusOK, map[string]any{"schema": 1, "templates": []any{}})
			return
		}
		http.NotFound(w, r)
	default:
		// A host nobody planned for is a request the test build should not make
		http.Error(w, "fakeupstream: no answer planned for the host "+host, http.StatusBadGateway)
	}
}

func (s *Server) scryfall(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/cards/search":
		matches, err := s.search(r.URL.Query().Get("q"))
		if err != nil {
			scryfallError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if len(matches) == 0 {
			scryfallError(w, http.StatusNotFound, "not_found", "Your query didn't match any cards.")
			return
		}
		list := make([]json.RawMessage, len(matches))
		for i, m := range matches {
			list[i] = m.raw
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "total_cards": len(list), "has_more": false, "data": list})
	case r.URL.Path == "/cards/named":
		q := r.URL.Query()
		name := strings.ToLower(q.Get("exact") + q.Get("fuzzy"))
		for _, c := range s.cards {
			if strings.ToLower(c.name) == name || (q.Get("fuzzy") != "" && strings.Contains(strings.ToLower(c.name), name)) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(c.raw)
				return
			}
		}
		scryfallError(w, http.StatusNotFound, "not_found", "No cards found matching \""+name+"\"")
	case r.URL.Path == "/bulk-data":
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "has_more": false, "data": []any{}})
	default:
		scryfallError(w, http.StatusNotFound, "not_found", "fakeupstream does not serve "+r.URL.Path)
	}
}

// search understands a small part of Scryfall's syntax, enough for the flows
// the tests drive: words that must appear in a name, !"an exact name", t:word
// for the type line, set:code, and unique:prints, which changes nothing since
// every printing is a file of its own. Any other operator is an error, so a
// test never passes on a query the fake answered wrongly
func (s *Server) search(q string) ([]card, error) {
	var words, types, sets []string
	exact := ""
	for _, tok := range tokens(q) {
		low := strings.ToLower(tok)
		switch {
		case strings.HasPrefix(low, "!\""):
			exact = strings.Trim(low[1:], "\"")
		case low == "unique:prints":
		case strings.HasPrefix(low, "t:"):
			types = append(types, strings.Trim(low[2:], "\""))
		case strings.HasPrefix(low, "set:"):
			sets = append(sets, low[4:])
		case strings.ContainsAny(low, ":=<>") && !strings.HasPrefix(low, "\""):
			return nil, fmt.Errorf("fakeupstream does not support the search term %q", tok)
		default:
			words = append(words, strings.Trim(low, "\""))
		}
	}
	var out []card
	for _, c := range s.cards {
		name := strings.ToLower(c.name)
		if exact != "" && name != exact && !strings.HasPrefix(name, exact+" //") {
			continue
		}
		if !all(name, words) || !all(strings.ToLower(c.typeLine), types) {
			continue
		}
		if len(sets) > 0 && !contains(sets, c.set) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// tokens splits a query on spaces, keeping a quoted phrase whole
func tokens(q string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func all(text string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(text, p) {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func scryfallError(w http.ResponseWriter, status int, code, details string) {
	writeJSON(w, status, map[string]any{"object": "error", "status": status, "code": code, "details": details})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// art answers an image request with a JPEG that depends only on the path, so a
// card shows the same art every run and two cards show different art
func art(w http.ResponseWriter, r *http.Request) {
	h := fnv.New32a()
	h.Write([]byte(r.URL.Path))
	seed := h.Sum32()
	base := color.RGBA{R: uint8(seed), G: uint8(seed >> 8), B: uint8(seed >> 16), A: 255}
	const width, height = 626, 457
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			// A diagonal gradient from the base color to its inverse
			t := float64(x+y) / float64(width+height)
			img.SetRGBA(x, y, color.RGBA{
				R: lerp(base.R, 255-base.R, t),
				G: lerp(base.G, 255-base.G, t),
				B: lerp(base.B, 255-base.B, t),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(buf.Bytes())
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
