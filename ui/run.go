package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card"
)

// The states a card moves through in a run. Skipped is a card a Stop reached
// before it finished
const (
	cardQueued    = "queued"
	cardFetching  = "fetching"
	cardRendering = "rendering"
	cardWriting   = "writing"
	cardDone      = "done"
	cardFailed    = "failed"
	cardSkipped   = "skipped"
)

// defaultConcurrency is how many cards render at once when nothing is set. Each
// full-size render holds a buffer of tens of megabytes, so memory rather than
// CPU is what bounds it
const (
	defaultConcurrency = 2
	maxConcurrency     = 6
)

// runRow is one card of a run as the review table sends it: the resolved card,
// any field overrides on top, and how many copies the list asked for
type runRow struct {
	Name   string            `json:"name"`
	Qty    int               `json:"qty"`
	Group  string            `json:"group,omitempty"`
	Base   card.Data         `json:"base"`
	Fields map[string]string `json:"fields,omitempty"`
}

// runCard is one card's progress, sent whole on every change so an event never
// depends on the ones before it
type runCard struct {
	Index  int     `json:"index"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Stage  string  `json:"stage,omitempty"`
	Err    string  `json:"error,omitempty"`
	Frac   float64 `json:"frac,omitempty"`
	File   string  `json:"file,omitempty"`
	Millis int64   `json:"ms,omitempty"`
}

// batchRun is one batch render: its rows, where they go, and each card's state.
// The server keeps the latest run until another starts, so a finished run stays
// readable in the console
type batchRun struct {
	id          string
	job         *job
	cancel      context.CancelFunc
	outDir      string
	label       string
	template    string
	version     string
	dpi         int
	concurrency int
	started     time.Time
	rows        []runRow
	files       []string

	mu       sync.Mutex
	cards    []runCard
	finished time.Time
	stopped  bool
	report   string
}

// runView is the snapshot GET /api/run returns, enough to draw the console for
// a page that loaded after the run began
type runView struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	OutDir      string    `json:"outDir"`
	Template    string    `json:"template"`
	DPI         int       `json:"dpi"`
	Concurrency int       `json:"concurrency"`
	Started     time.Time `json:"started"`
	Finished    time.Time `json:"finished,omitzero"`
	Stopped     bool      `json:"stopped,omitempty"`
	Report      string    `json:"report,omitempty"`
	Cards       []runCard `json:"cards"`
}

func (r *batchRun) view() runView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return runView{
		ID:          r.id,
		Label:       r.label,
		OutDir:      r.outDir,
		Template:    strings.TrimSpace(r.template + " " + r.version),
		DPI:         r.dpi,
		Concurrency: r.concurrency,
		Started:     r.started,
		Finished:    r.finished,
		Stopped:     r.stopped,
		Report:      r.report,
		Cards:       append([]runCard(nil), r.cards...),
	}
}

func (r *batchRun) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished.IsZero()
}

// update changes one card under the lock and emits the result with the run's
// overall progress, plus a log line when there is one
func (r *batchRun) update(i int, logLine string, fn func(c *runCard)) {
	r.mu.Lock()
	fn(&r.cards[i])
	c := r.cards[i]
	settled := 0
	for _, x := range r.cards {
		if x.Status == cardDone || x.Status == cardFailed || x.Status == cardSkipped {
			settled++
		}
	}
	r.mu.Unlock()
	e := jobEvent{
		Step: fmt.Sprintf("%d of %d", settled, len(r.cards)),
		Frac: float64(settled) / float64(len(r.cards)),
		Card: &c,
	}
	if logLine != "" {
		e.Log = time.Now().Format("15:04:05") + "  " + logLine
	}
	r.job.emit(e)
}

func (r *batchRun) log(line string) {
	r.job.emit(jobEvent{Log: time.Now().Format("15:04:05") + "  " + line})
}

// runBody is the POST /api/run payload
type runBody struct {
	Rows   []runRow `json:"rows"`
	OutDir string   `json:"outDir"`
	Label  string   `json:"label"`
}

// currentRun returns the latest run, which may have finished
func (s *server) currentRun() *batchRun {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.run
}

// runActive reports whether a batch is rendering now
func (s *server) runActive() bool {
	r := s.currentRun()
	return r != nil && r.active()
}

// handleRun starts a batch. It is refused while another is running, since two
// runs writing into overlapping folders is a class of bug better designed out
// than handled
func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	var body runBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad run request: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.startRun(w, body.Rows, body.OutDir, body.Label)
}

// handleRetryRun starts a fresh run from the cards that failed in the latest
// one, into the same folder, which is the usual fix after a network blip
func (s *server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	prev, ok := s.runByID(w, r)
	if !ok {
		return
	}
	var rows []runRow
	prev.mu.Lock()
	for i, c := range prev.cards {
		if c.Status == cardFailed {
			rows = append(rows, prev.rows[i])
		}
	}
	prev.mu.Unlock()
	label := prev.label
	if label != "" {
		label += ", retried"
	}
	s.startRun(w, rows, prev.outDir, label)
}

// startRun validates a run and starts it, answering with its first snapshot
func (s *server) startRun(w http.ResponseWriter, rows []runRow, outDir, label string) {
	if len(rows) == 0 {
		http.Error(w, "the run has no cards", http.StatusBadRequest)
		return
	}
	if len(rows) > maxListRows {
		http.Error(w, fmt.Sprintf("a run takes at most %d cards", maxListRows), http.StatusBadRequest)
		return
	}
	dir, err := prepareOutDir(outDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dpi := s.renderDPI(targetOutput)
	if m, err := s.pipe.manifest(); err == nil {
		dpi = m.ClampDPI(dpi)
	}
	name, version := s.active()
	ctx, cancel := context.WithCancel(context.Background())
	run := &batchRun{
		cancel:      cancel,
		outDir:      dir,
		label:       strings.TrimSpace(label),
		template:    name,
		version:     version,
		dpi:         dpi,
		concurrency: concurrencyOf(s.prefs.settings()),
		started:     time.Now(),
		rows:        rows,
		files:       outputNames(rows),
		cards:       make([]runCard, len(rows)),
		job:         &job{created: time.Now()},
	}
	for i, row := range rows {
		run.cards[i] = runCard{Index: i, Name: row.displayName(), Status: cardQueued}
	}

	s.runMu.Lock()
	if s.run != nil && s.run.active() {
		s.runMu.Unlock()
		cancel()
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}
	s.runSeq++
	run.id = "run-" + strconv.FormatUint(s.runSeq, 10)
	s.run = run
	s.runMu.Unlock()

	go s.execute(ctx, run)
	writeJSON(w, run.view())
}

// handleLatestRun returns the latest run, or 204 when there has been none
func (s *server) handleLatestRun(w http.ResponseWriter, r *http.Request) {
	run := s.currentRun()
	if run == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, run.view())
}

// runByID returns the latest run when id names it. Earlier runs are gone
func (s *server) runByID(w http.ResponseWriter, r *http.Request) (*batchRun, bool) {
	run := s.currentRun()
	if run == nil || run.id != r.PathValue("id") {
		http.NotFound(w, r)
		return nil, false
	}
	return run, true
}

func (s *server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if run, ok := s.runByID(w, r); ok {
		streamJob(w, r, run.job)
	}
}

// handleRunImage serves one finished card from the file the run wrote, so a
// run of hundreds of cards holds none of them in memory
func (s *server) handleRunImage(w http.ResponseWriter, r *http.Request) {
	run, ok := s.runByID(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 || n >= len(run.rows) {
		http.NotFound(w, r)
		return
	}
	run.mu.Lock()
	c := run.cards[n]
	run.mu.Unlock()
	if c.Status != cardDone {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(run.outDir, c.File))
}

func (s *server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	run, ok := s.runByID(w, r)
	if !ok {
		return
	}
	run.mu.Lock()
	if run.finished.IsZero() {
		run.stopped = true
	}
	run.mu.Unlock()
	run.cancel()
	w.WriteHeader(http.StatusNoContent)
}

// handleOpenRunFolder opens the run's output folder in the system file browser.
// It opens only a folder a run wrote to, never a path the request names
func (s *server) handleOpenRunFolder(w http.ResponseWriter, r *http.Request) {
	run, ok := s.runByID(w, r)
	if !ok {
		return
	}
	if err := openBrowser(run.outDir); err != nil {
		http.Error(w, "opening the folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// execute renders every card with a small worker pool, writes the report, and
// ends the stream. A Stop cancels ctx: queued cards are skipped at once and a
// card mid-render stops at the engine's next cancellation check
func (s *server) execute(ctx context.Context, run *batchRun) {
	run.log(fmt.Sprintf("run %s: %d cards with %s at %d dpi, %d at a time, into %s",
		run.id, len(run.rows), strings.TrimSpace(run.template+" "+run.version), run.dpi, run.concurrency, run.outDir))

	next := make(chan int)
	var wg sync.WaitGroup
	for range run.concurrency {
		wg.Go(func() {
			for i := range next {
				s.renderRunCard(ctx, run, i)
			}
		})
	}
	for i := range run.rows {
		if ctx.Err() != nil {
			break
		}
		select {
		case next <- i:
		case <-ctx.Done():
		}
	}
	close(next)
	wg.Wait()

	run.mu.Lock()
	var skipped []int
	for i, c := range run.cards {
		if c.Status == cardQueued {
			skipped = append(skipped, i)
		}
	}
	run.mu.Unlock()
	for _, i := range skipped {
		run.update(i, "", func(c *runCard) { c.Status = cardSkipped })
	}

	run.mu.Lock()
	run.finished = time.Now()
	run.mu.Unlock()
	if path, err := writeReport(run); err != nil {
		run.log("report not written: " + err.Error())
	} else {
		run.mu.Lock()
		run.report = filepath.Base(path)
		run.mu.Unlock()
		run.log("report written to " + filepath.Base(path))
	}
	v := run.view()
	counts := countCards(v.Cards)
	summary := fmt.Sprintf("%d done, %d failed, %d skipped in %s", counts[cardDone], counts[cardFailed], counts[cardSkipped],
		v.Finished.Sub(v.Started).Round(100*time.Millisecond))
	run.log("finished: " + summary)
	run.job.emit(jobEvent{Done: true, Frac: 1, Step: summary})
}

// renderRunCard fetches art for one card, renders it at the run's resolution,
// and writes the PNG, reporting each stage
func (s *server) renderRunCard(ctx context.Context, run *batchRun, i int) {
	if ctx.Err() != nil {
		return
	}
	row := run.rows[i]
	name := row.displayName()
	start := time.Now()
	fail := func(stage string, err error) {
		if ctx.Err() != nil {
			run.update(i, name+": stopped", func(c *runCard) { c.Status = cardSkipped; c.Frac = 0 })
			return
		}
		run.update(i, fmt.Sprintf("%s: %s failed: %v", name, stage, err), func(c *runCard) {
			c.Status, c.Stage, c.Err, c.Frac = cardFailed, stage, err.Error(), 0
		})
	}

	d := overlayFields(&row.Base, row.Fields)
	run.update(i, "", func(c *runCard) { c.Status = cardFetching })
	var art image.Image
	if row.Base.ArtworkURL != "" {
		artCtx, cancel := context.WithTimeout(ctx, netTimeout)
		img, err := s.pipe.fetchArt(artCtx, &row.Base)
		cancel()
		if err != nil {
			fail("art", err)
			return
		}
		art = img
	}

	run.update(i, "", func(c *runCard) { c.Status = cardRendering })
	renderCtx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	last := 0.0
	img, err := s.pipe.render(renderCtx, d, art, run.dpi, func(step string, frac float64) {
		// A tenth at a time keeps a long run's event backlog short
		if frac-last < 0.1 && frac < 1 {
			return
		}
		last = frac
		run.update(i, "", func(c *runCard) { c.Frac = frac })
	})
	if err != nil {
		fail("render", err)
		return
	}

	run.update(i, "", func(c *runCard) { c.Status = cardWriting })
	file := run.files[i]
	if err := writePNG(filepath.Join(run.outDir, file), img); err != nil {
		fail("write", err)
		return
	}
	ms := time.Since(start).Milliseconds()
	run.update(i, fmt.Sprintf("%s: wrote %s in %.1fs", name, file, float64(ms)/1000), func(c *runCard) {
		c.Status, c.File, c.Millis, c.Frac = cardDone, file, ms, 1
	})
}

func (row runRow) displayName() string {
	if n := row.Fields["name"]; n != "" {
		return n
	}
	if row.Base.Name != "" {
		return row.Base.Name
	}
	return row.Name
}

func countCards(cards []runCard) map[string]int {
	m := make(map[string]int)
	for _, c := range cards {
		m[c.Status]++
	}
	return m
}

// concurrencyOf reads the render concurrency setting, filling in the default
// and keeping it in a range the memory of one machine can hold
func concurrencyOf(s uiSettings) int {
	if s.Concurrency <= 0 {
		return defaultConcurrency
	}
	return min(s.Concurrency, maxConcurrency)
}

// prepareOutDir resolves the output folder, creating it when it does not exist
// yet, and checks it can be written before any card renders
func prepareOutDir(dir string) (string, error) {
	dir, err := expandPath(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating the output folder: %w", err)
	}
	probe, err := os.CreateTemp(dir, ".mimic-probe-*")
	if err != nil {
		return "", fmt.Errorf("the output folder is not writable: %w", err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return dir, nil
}

// expandPath turns a typed path into a clean absolute one, reading a leading ~
// as the home folder. A relative path is refused, since relative to the
// server's working directory is never what a user means
func expandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("choose an output folder")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%q is not a full path", p)
	}
	return filepath.Clean(p), nil
}

// outputNames picks every card's filename up front, in list order, so two cards
// that would share a name get the same suffix however the workers interleave.
// A name carries the set code and collector number of the printing pulled from
// Scryfall, and a rerun into the same folder overwrites the files it wrote
func outputNames(rows []runRow) []string {
	names := make([]string, len(rows))
	seen := make(map[string]int)
	for i, row := range rows {
		stem := fileStem(row.displayName(), row.Base.SetCode, row.Base.CollectorNumber)
		key := strings.ToLower(stem)
		seen[key]++
		if n := seen[key]; n > 1 {
			stem = fmt.Sprintf("%s (%d)", stem, n)
		}
		names[i] = stem + ".png"
	}
	return names
}

// fileStem builds a filename without its extension: the card name, then the
// printing in brackets when there is one, as in Sol Ring [C21-263]
func fileStem(name, set, number string) string {
	stem := sanitizeFilename(name)
	if stem == "" {
		stem = "Untitled"
	}
	set, number = sanitizeFilename(set), sanitizeFilename(number)
	switch {
	case set != "" && number != "":
		stem += fmt.Sprintf(" [%s-%s]", strings.ToUpper(set), number)
	case set != "":
		stem += fmt.Sprintf(" [%s]", strings.ToUpper(set))
	}
	return stem
}

// windowsReserved are names Windows refuses for a file whatever the extension
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeFilename makes a card name safe as a filename on every OS. A split
// card's // becomes a dash, characters no filesystem accepts are dropped, and
// the result is bounded in length
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, " // ", " - ")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), ". ")
	if len(out) > 150 {
		out = strings.TrimSpace(out[:150])
	}
	if windowsReserved[strings.ToUpper(out)] {
		out = "_" + out
	}
	return out
}

// writePNG encodes img to path through a temporary file in the same folder, so
// a crash or a Stop never leaves a half-written PNG under the real name
func writePNG(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mimic-*.png")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	bw := bufio.NewWriterSize(tmp, 1<<20)
	if err := png.Encode(bw, img); err != nil {
		tmp.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file private, and output is an ordinary user file
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// runReport is the JSON written beside the PNGs, enough to see what a run did
// and to reproduce it
type runReport struct {
	ID          string         `json:"id"`
	Label       string         `json:"label,omitempty"`
	Started     time.Time      `json:"started"`
	Finished    time.Time      `json:"finished"`
	Stopped     bool           `json:"stopped,omitempty"`
	Template    string         `json:"template"`
	DPI         int            `json:"dpi"`
	Concurrency int            `json:"concurrency"`
	Counts      map[string]int `json:"counts"`
	Cards       []reportCard   `json:"cards"`
}

type reportCard struct {
	Name      string            `json:"name"`
	Qty       int               `json:"qty"`
	Group     string            `json:"group,omitempty"`
	Set       string            `json:"set,omitempty"`
	Collector string            `json:"collector,omitempty"`
	Status    string            `json:"status"`
	Stage     string            `json:"stage,omitempty"`
	Error     string            `json:"error,omitempty"`
	File      string            `json:"file,omitempty"`
	Millis    int64             `json:"ms,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// writeReport writes mimic-run-<timestamp>.json into the run's folder
func writeReport(run *batchRun) (string, error) {
	v := run.view()
	rep := runReport{
		ID:          v.ID,
		Label:       v.Label,
		Started:     v.Started,
		Finished:    v.Finished,
		Stopped:     v.Stopped,
		Template:    v.Template,
		DPI:         v.DPI,
		Concurrency: v.Concurrency,
		Counts:      countCards(v.Cards),
	}
	for i, c := range v.Cards {
		row := run.rows[i]
		rep.Cards = append(rep.Cards, reportCard{
			Name:      c.Name,
			Qty:       max(row.Qty, 1),
			Group:     row.Group,
			Set:       row.Base.SetCode,
			Collector: row.Base.CollectorNumber,
			Status:    c.Status,
			Stage:     c.Stage,
			Error:     c.Err,
			File:      c.File,
			Millis:    c.Millis,
			Fields:    row.Fields,
		})
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(run.outDir, "mimic-run-"+v.Started.Format("20060102-150405")+".json")
	return path, os.WriteFile(path, raw, 0o644)
}
