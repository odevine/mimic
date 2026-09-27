// Package batch renders a list of cards into a folder of PNGs with a small
// worker pool, tracking each card's progress and writing a report beside them
package batch

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// The states a card moves through in a run. Skipped is a card a Stop reached
// before it finished. Unsupported is a card the run's template does not render,
// which no retry fixes
const (
	StatusQueued      = "queued"
	StatusFetching    = "fetching"
	StatusRendering   = "rendering"
	StatusWriting     = "writing"
	StatusDone        = "done"
	StatusFailed      = "failed"
	StatusSkipped     = "skipped"
	StatusUnsupported = "unsupported"
)

// settledStatus reports whether a card has finished moving through the run
func settledStatus(status string) bool {
	return status == StatusDone || status == StatusFailed || status == StatusSkipped || status == StatusUnsupported
}

// defaultConcurrency is how many cards render at once when nothing is set, and
// MaxConcurrency the most a setting may ask for. Each full-size render holds a
// buffer of tens of megabytes, so memory rather than CPU is what bounds it
const (
	defaultConcurrency = 2
	MaxConcurrency     = 6
)

// Row is one card of a run as the review table sends it: the resolved card,
// any field overrides on top, and how many copies the list asked for. A run
// renders each face of a card as its own row, and Face picks which
type Row struct {
	Name   string            `json:"name"`
	Qty    int               `json:"qty"`
	Group  string            `json:"group,omitempty"`
	Base   card.Data         `json:"base"`
	Fields map[string]string `json:"fields,omitempty"`
	Face   int               `json:"face,omitempty"`
}

// Card is one card's progress, sent whole on every change so an event never
// depends on the ones before it
type Card struct {
	Index  int     `json:"index"`
	Name   string  `json:"name"`
	Face   int     `json:"face,omitempty"`
	Status string  `json:"status"`
	Stage  string  `json:"stage,omitempty"`
	Err    string  `json:"error,omitempty"`
	Frac   float64 `json:"frac,omitempty"`
	File   string  `json:"file,omitempty"`
	Millis int64   `json:"ms,omitempty"`
}

// Event is one progress message from a run. Step and Frac are the run's
// overall progress, Card is a card that changed, Log is a console line, and
// Done marks the last event
type Event struct {
	Step string
	Frac float64
	Card *Card
	Log  string
	Done bool
}

// Options describes a run apart from its rows
type Options struct {
	ID          string
	OutDir      string
	Label       string
	Template    string
	Version     string
	DPI         int
	Concurrency int
	// ArtTimeout bounds one art download and RenderTimeout one card's render
	ArtTimeout    time.Duration
	RenderTimeout time.Duration
	// Emit receives every progress event. It is called from several goroutines
	Emit func(Event)
}

// Run is one batch render: its rows, where they go, and each card's state. The
// server keeps the latest run until another starts, so a finished run stays
// readable in the console
type Run struct {
	opts    Options
	ctx     context.Context
	cancel  context.CancelFunc
	started time.Time
	rows    []Row
	files   []string

	mu       sync.Mutex
	cards    []Card
	finished time.Time
	stopped  bool
	report   string
}

// New prepares a run of rows, with every card queued. Nothing renders until
// Execute
func New(rows []Row, o Options) *Run {
	if o.Emit == nil {
		o.Emit = func(Event) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Run{
		opts:    o,
		ctx:     ctx,
		cancel:  cancel,
		started: time.Now(),
		rows:    rows,
		files:   outputNames(rows),
		cards:   make([]Card, len(rows)),
	}
	for i, row := range rows {
		r.cards[i] = Card{Index: i, Name: row.displayName(), Face: row.Face, Status: StatusQueued}
	}
	return r
}

// ID names the run
func (r *Run) ID() string { return r.opts.ID }

// OutDir is the folder the run writes into
func (r *Run) OutDir() string { return r.opts.OutDir }

// View is a snapshot of a run, enough to draw the console for a page that
// loaded after the run began
type View struct {
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
	Cards       []Card    `json:"cards"`
}

// View returns a snapshot of the run
func (r *Run) View() View {
	r.mu.Lock()
	defer r.mu.Unlock()
	return View{
		ID:          r.opts.ID,
		Label:       r.opts.Label,
		OutDir:      r.opts.OutDir,
		Template:    strings.TrimSpace(r.opts.Template + " " + r.opts.Version),
		DPI:         r.opts.DPI,
		Concurrency: r.opts.Concurrency,
		Started:     r.started,
		Finished:    r.finished,
		Stopped:     r.stopped,
		Report:      r.report,
		Cards:       append([]Card(nil), r.cards...),
	}
}

// Active reports whether the run has not finished yet
func (r *Run) Active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished.IsZero()
}

// Stop cancels the run. Queued cards are skipped at once and a card mid-render
// stops at the engine's next cancellation check
func (r *Run) Stop() {
	r.mu.Lock()
	if r.finished.IsZero() {
		r.stopped = true
	}
	r.mu.Unlock()
	r.cancel()
}

// File is the path of card n's PNG, once it is written
func (r *Run) File(n int) (string, bool) {
	if n < 0 || n >= len(r.rows) {
		return "", false
	}
	r.mu.Lock()
	c := r.cards[n]
	r.mu.Unlock()
	if c.Status != StatusDone {
		return "", false
	}
	return filepath.Join(r.opts.OutDir, c.File), true
}

// Failed returns the rows whose cards failed, which is what a retry renders
func (r *Run) Failed() []Row {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rows []Row
	for i, c := range r.cards {
		if c.Status == StatusFailed {
			rows = append(rows, r.rows[i])
		}
	}
	return rows
}

// Label is the run's own label
func (r *Run) Label() string { return r.opts.Label }

// update changes one card under the lock and emits the result with the run's
// overall progress, plus a log line when there is one
func (r *Run) update(i int, logLine string, fn func(c *Card)) {
	r.mu.Lock()
	fn(&r.cards[i])
	c := r.cards[i]
	settled := 0
	for _, x := range r.cards {
		if settledStatus(x.Status) {
			settled++
		}
	}
	r.mu.Unlock()
	e := Event{
		Step: fmt.Sprintf("%d of %d", settled, len(r.cards)),
		Frac: float64(settled) / float64(len(r.cards)),
		Card: &c,
	}
	if logLine != "" {
		e.Log = time.Now().Format("15:04:05") + "  " + logLine
	}
	r.opts.Emit(e)
}

func (r *Run) log(line string) {
	r.opts.Emit(Event{Log: time.Now().Format("15:04:05") + "  " + line})
}

// Execute renders every card through p with a small worker pool, writes the
// report, and emits the last event. A Stop ends it early
func (r *Run) Execute(p *pipeline.Pipeline) {
	defer r.cancel()
	ctx := r.ctx
	r.log(fmt.Sprintf("run %s: %d cards with %s at %d dpi, %d at a time, into %s",
		r.opts.ID, len(r.rows), strings.TrimSpace(r.opts.Template+" "+r.opts.Version), r.opts.DPI, r.opts.Concurrency, r.opts.OutDir))

	next := make(chan int)
	var wg sync.WaitGroup
	for range max(r.opts.Concurrency, 1) {
		wg.Go(func() {
			for i := range next {
				r.renderCard(ctx, p, i)
			}
		})
	}
	for i := range r.rows {
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

	r.mu.Lock()
	var skipped []int
	for i, c := range r.cards {
		if c.Status == StatusQueued {
			skipped = append(skipped, i)
		}
	}
	r.mu.Unlock()
	for _, i := range skipped {
		r.update(i, "", func(c *Card) { c.Status = StatusSkipped })
	}

	r.mu.Lock()
	r.finished = time.Now()
	r.mu.Unlock()
	if path, err := writeReport(r); err != nil {
		r.log("report not written: " + err.Error())
	} else {
		r.mu.Lock()
		r.report = filepath.Base(path)
		r.mu.Unlock()
		r.log("report written to " + filepath.Base(path))
	}
	v := r.View()
	counts := Counts(v.Cards)
	summary := fmt.Sprintf("%d done, %d failed, %d skipped", counts[StatusDone], counts[StatusFailed], counts[StatusSkipped])
	if n := counts[StatusUnsupported]; n > 0 {
		summary += fmt.Sprintf(", %d unsupported", n)
	}
	summary += " in " + v.Finished.Sub(v.Started).Round(100*time.Millisecond).String()
	r.log("finished: " + summary)
	r.opts.Emit(Event{Done: true, Frac: 1, Step: summary})
}

// renderCard fetches art for one card, renders it at the run's resolution, and
// writes the PNG, reporting each stage
func (r *Run) renderCard(ctx context.Context, p *pipeline.Pipeline, i int) {
	if ctx.Err() != nil {
		return
	}
	row := r.rows[i]
	name := row.displayName()
	start := time.Now()
	fail := func(stage string, err error) {
		if ctx.Err() != nil {
			r.update(i, name+": stopped", func(c *Card) { c.Status = StatusSkipped; c.Frac = 0 })
			return
		}
		if pipeline.IsUnsupported(err) {
			r.update(i, name+": "+err.Error(), func(c *Card) {
				c.Status, c.Stage, c.Err, c.Frac = StatusUnsupported, stage, err.Error(), 0
			})
			return
		}
		r.update(i, fmt.Sprintf("%s: %s failed: %v", name, stage, err), func(c *Card) {
			c.Status, c.Stage, c.Err, c.Frac = StatusFailed, stage, err.Error(), 0
		})
	}

	d := cardlist.Overlay(&row.Base, row.Fields)
	// Checked before the art download, which would be wasted on a face no
	// template renders
	if err := p.Unsupported(d, row.Face); err != nil {
		fail("check", err)
		return
	}
	r.update(i, "", func(c *Card) { c.Status = StatusFetching })
	var art image.Image
	if faceBase := row.Base.Face(row.Face); faceBase.ArtworkURL != "" {
		artCtx, cancel := context.WithTimeout(ctx, r.opts.ArtTimeout)
		img, err := p.FetchArt(artCtx, faceBase)
		cancel()
		if err != nil {
			fail("art", err)
			return
		}
		art = img
	}

	r.update(i, "", func(c *Card) { c.Status = StatusRendering })
	renderCtx, cancel := context.WithTimeout(ctx, r.opts.RenderTimeout)
	defer cancel()
	last := 0.0
	img, err := p.Render(renderCtx, d, row.Face, art, r.opts.DPI, func(step string, frac float64) {
		// A tenth at a time keeps a long run's event backlog short
		if frac-last < 0.1 && frac < 1 {
			return
		}
		last = frac
		r.update(i, "", func(c *Card) { c.Frac = frac })
	})
	if err != nil {
		fail("render", err)
		return
	}

	r.update(i, "", func(c *Card) { c.Status = StatusWriting })
	file := r.files[i]
	if err := writePNG(filepath.Join(r.opts.OutDir, file), img); err != nil {
		fail("write", err)
		return
	}
	ms := time.Since(start).Milliseconds()
	r.update(i, fmt.Sprintf("%s: wrote %s in %.1fs", name, file, float64(ms)/1000), func(c *Card) {
		c.Status, c.File, c.Millis, c.Frac = StatusDone, file, ms, 1
	})
}

// displayName is the name the face prints, or the edited name for a front
func (row Row) displayName() string {
	if row.Face > 0 {
		if n := row.Base.Face(row.Face).Name; n != "" {
			return n
		}
	}
	if n := row.Fields["name"]; n != "" {
		return n
	}
	if row.Base.Name != "" {
		return row.Base.Name
	}
	return row.Name
}

// Counts tallies cards by status
func Counts(cards []Card) map[string]int {
	m := make(map[string]int)
	for _, c := range cards {
		m[c.Status]++
	}
	return m
}

// Concurrency reads the render concurrency setting, filling in the default and
// keeping it in a range the memory of one machine can hold
func Concurrency(setting int) int {
	if setting <= 0 {
		return defaultConcurrency
	}
	return min(setting, MaxConcurrency)
}

// ExpandFaces turns each card the list sends into one row per image it renders
// to, so a double-faced card renders its front and back as two cards with their
// own files. The faces follow their card in list order
func ExpandFaces(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		row.Face = 0
		out = append(out, row)
		n := len(template.Classify(cardlist.Overlay(&row.Base, row.Fields)))
		for f := 1; f < n; f++ {
			back := row
			back.Face = f
			out = append(out, back)
		}
	}
	return out
}
