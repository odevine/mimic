// Package httpapi serves the services over loopback HTTP and the frontend files
// beside them, so the web build keeps working while the services are moved
// behind a desktop shell. It adds no behavior of its own
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/fontdir"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/services/cards"
	"github.com/odevine/mimic/ui/internal/services/data"
	"github.com/odevine/mimic/ui/internal/services/list"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/services/templates"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Services is every service the routes call
type Services struct {
	Workspace *workspace.Workspace
	Cards     *cards.Service
	Render    *render.Service
	List      *list.Service
	Run       *run.Service
	Templates *templates.Service
	Settings  *settings.Service
	Data      *data.Service
}

// Server is the HTTP face of the services
type Server struct {
	Services
	mux *http.ServeMux
	// static is the frontend served at the site root
	static fs.FS
}

// New wires the routes. static is the frontend served at the site root
func New(svc Services, static fs.FS) *Server {
	s := &Server{Services: svc, static: static}
	s.routes()
	return s
}

// Handler is the routes behind the loopback host guard
func (s *Server) Handler() http.Handler { return guard(s.mux) }

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/recents", s.recents)
	mux.HandleFunc("POST /api/render", s.startRender)
	mux.HandleFunc("GET /api/render/{id}/events", s.jobEvents)
	mux.HandleFunc("GET /api/render/{id}/image", s.renderImage)
	mux.HandleFunc("POST /api/render/{id}/cancel", s.cancelRender)
	mux.HandleFunc("GET /api/resolution", s.resolution)
	mux.HandleFunc("GET /api/resources", s.resources)
	mux.HandleFunc("POST /api/resolution", s.setResolution)
	mux.HandleFunc("GET /api/templates", s.templateList)
	mux.HandleFunc("GET /api/template/active", s.activeTemplate)
	mux.HandleFunc("POST /api/template/select", s.selectTemplate)
	mux.HandleFunc("GET /api/template/select/{id}/events", s.jobEvents)
	mux.HandleFunc("GET /api/template/faces", s.faces)
	mux.HandleFunc("PUT /api/template/faces", s.setFace)
	mux.HandleFunc("GET /api/capabilities", s.capabilities)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/printings", s.printings)
	mux.HandleFunc("GET /api/symbol", s.symbol)
	mux.HandleFunc("POST /api/resolve", s.resolve)
	mux.HandleFunc("GET /api/resolve/{id}/events", s.jobEvents)
	mux.HandleFunc("POST /api/run", s.startRun)
	mux.HandleFunc("GET /api/run", s.latestRun)
	mux.HandleFunc("GET /api/run/{id}/events", s.runEvents)
	mux.HandleFunc("GET /api/run/{id}/image/{n}", s.runImage)
	mux.HandleFunc("POST /api/run/{id}/stop", s.stopRun)
	mux.HandleFunc("POST /api/run/{id}/retry", s.retryRun)
	mux.HandleFunc("POST /api/run/{id}/open", s.openRunFolder)
	mux.HandleFunc("GET /api/fs/list", s.fsList)
	mux.HandleFunc("GET /api/mpc", s.mpc)
	mux.HandleFunc("GET /api/mpc/cardback", s.cardbackImage)
	mux.HandleFunc("PUT /api/mpc/cardback", s.putCardback)
	mux.HandleFunc("GET /api/mpc/folder", s.mpcFolder)
	mux.HandleFunc("GET /api/carddata", s.cardData)
	mux.HandleFunc("POST /api/carddata/download", s.downloadCardData)
	mux.HandleFunc("GET /api/carddata/{id}/events", s.jobEvents)
	mux.HandleFunc("DELETE /api/carddata", s.removeCardData)
	mux.HandleFunc("GET /api/fonts", s.fonts)
	mux.HandleFunc("PUT /api/fonts/{folder}", s.putFont)
	mux.HandleFunc("DELETE /api/fonts/{folder}", s.deleteFont)
	mux.HandleFunc("POST /api/fonts/open", s.openFonts)
	if s.static != nil {
		mux.Handle("/", http.FileServerFS(s.static))
	}
	s.mux = mux
}

// statuses maps a service error's kind to the HTTP status that carried it
var statuses = map[apierr.Kind]int{
	apierr.Internal:   http.StatusInternalServerError,
	apierr.BadRequest: http.StatusBadRequest,
	apierr.NotFound:   http.StatusNotFound,
	apierr.Conflict:   http.StatusConflict,
	apierr.BadGateway: http.StatusBadGateway,
	apierr.TooLarge:   http.StatusRequestEntityTooLarge,
}

// fail answers with the status for err's kind and its message. A request the
// client abandoned gets no answer
func fail(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	http.Error(w, err.Error(), statuses[apierr.KindOf(err)])
}

// writeJSON encodes v as the JSON response body
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads the JSON request body into v, answering 400 when it is not valid
func decode(w http.ResponseWriter, r *http.Request, v any, what string) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad "+what+" request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// noContent answers an action with no body
func noContent(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// result answers with v as JSON, or with err
func result(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	v, err := s.Cards.Search(r.Context(), r.URL.Query().Get("q"))
	result(w, v, err)
}

func (s *Server) printings(w http.ResponseWriter, r *http.Request) {
	v, err := s.Cards.Printings(r.Context(), r.URL.Query().Get("name"))
	result(w, v, err)
}

func (s *Server) recents(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Cards.Recents()) }

func (s *Server) startRender(w http.ResponseWriter, r *http.Request) {
	var req render.Request
	if !decode(w, r, &req, "render") {
		return
	}
	v, err := s.Render.Start(req)
	result(w, v, err)
}

func (s *Server) cancelRender(w http.ResponseWriter, r *http.Request) {
	noContent(w, s.Render.Cancel(r.PathValue("id")))
}

// renderImage writes a finished render's PNG. With a download query it sets an
// attachment disposition and the card-name filename, so the Save link downloads,
// and otherwise it serves inline for the preview image
func (s *Server) renderImage(w http.ResponseWriter, r *http.Request) {
	img, name, err := s.Render.Image(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", render.Filename(name)))
	}
	if err := writePNG(w, img); err != nil {
		http.Error(w, "encoding image: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) resolution(w http.ResponseWriter, r *http.Request) {
	v, err := s.Settings.Resolution()
	result(w, v, err)
}

func (s *Server) setResolution(w http.ResponseWriter, r *http.Request) {
	var req settings.ResolutionRequest
	if !decode(w, r, &req, "resolution") {
		return
	}
	v, err := s.Settings.SetResolution(req)
	result(w, v, err)
}

func (s *Server) resources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Settings.Resources(r.URL.Query().Get("cache")))
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Settings.Capabilities())
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Settings.Get()) }

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in prefs.Settings
	if !decode(w, r, &in, "settings") {
		return
	}
	writeJSON(w, s.Settings.Put(in))
}

func (s *Server) templateList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Templates.List(r.Context()))
}

func (s *Server) activeTemplate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Templates.Active())
}

func (s *Server) selectTemplate(w http.ResponseWriter, r *http.Request) {
	var req templates.SelectRequest
	if !decode(w, r, &req, "select") {
		return
	}
	id, err := s.Templates.Select(req)
	result(w, map[string]string{"jobId": id}, err)
}

func (s *Server) faces(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Templates.Faces()) }

func (s *Server) setFace(w http.ResponseWriter, r *http.Request) {
	var req templates.FaceChoice
	if !decode(w, r, &req, "face template") {
		return
	}
	v, err := s.Templates.SetFace(req)
	result(w, v, err)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var req list.Request
	if !decode(w, r, &req, "resolve") {
		return
	}
	v, err := s.List.Resolve(req)
	result(w, v, err)
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var req run.Request
	if !decode(w, r, &req, "run") {
		return
	}
	v, err := s.Run.Start(req)
	result(w, v, err)
}

// latestRun returns the latest run, or 204 when there has been none
func (s *Server) latestRun(w http.ResponseWriter, r *http.Request) {
	v := s.Run.Latest()
	if v == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, v)
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request) {
	v, err := s.Run.Retry(r.PathValue("id"))
	result(w, v, err)
}

func (s *Server) stopRun(w http.ResponseWriter, r *http.Request) {
	noContent(w, s.Run.Stop(r.PathValue("id")))
}

func (s *Server) openRunFolder(w http.ResponseWriter, r *http.Request) {
	noContent(w, s.Run.OpenFolder(r.PathValue("id")))
}

func (s *Server) runEvents(w http.ResponseWriter, r *http.Request) {
	j, err := s.Run.Job(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	streamJob(w, r, j)
}

// runImage serves one finished card from the file the run wrote
func (s *Server) runImage(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := s.Run.File(r.PathValue("id"), n)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

func (s *Server) mpc(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Run.MPCInfo()) }

func (s *Server) mpcFolder(w http.ResponseWriter, r *http.Request) {
	v, err := s.Run.MPCFolder(r.URL.Query().Get("path"))
	result(w, v, err)
}

// cardbackImage serves the stored cardback for the options thumbnail
func (s *Server) cardbackImage(w http.ResponseWriter, r *http.Request) {
	path, err := s.Run.CardbackFile()
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// putCardback stores the uploaded image as the cardback, named by the name query
// parameter
func (s *Server) putCardback(w http.ResponseWriter, r *http.Request) {
	v, err := s.Run.SetCardbackImage(r.URL.Query().Get("name"), r.Body)
	result(w, v, err)
}

func (s *Server) cardData(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Data.CardData(r.Context()))
}

func (s *Server) downloadCardData(w http.ResponseWriter, r *http.Request) {
	id, err := s.Data.DownloadCardData()
	result(w, map[string]string{"jobId": id}, err)
}

func (s *Server) removeCardData(w http.ResponseWriter, r *http.Request) {
	v, err := s.Data.RemoveCardData()
	result(w, v, err)
}

func (s *Server) fonts(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Data.Fonts()) }

// putFont stores an uploaded font for one role. The upload is a multipart form
// with the font in its "file" field
func (s *Server) putFont(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFontUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "reading the upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	v, err := s.Data.AddFontFile(r.PathValue("folder"), header.Filename, file)
	result(w, v, err)
}

func (s *Server) deleteFont(w http.ResponseWriter, r *http.Request) {
	v, err := s.Data.RemoveFont(r.PathValue("folder"))
	result(w, v, err)
}

func (s *Server) openFonts(w http.ResponseWriter, r *http.Request) { noContent(w, s.Data.OpenFonts()) }

// symbol draws one braced code as a PNG pip, with an ETag that carries the mana
// font's identity
func (s *Server) symbol(w http.ResponseWriter, r *http.Request) {
	px, _ := strconv.Atoi(r.URL.Query().Get("px"))
	img, tag, err := s.Data.Symbol(r.URL.Query().Get("code"), px, r.Header.Get("If-None-Match"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "no-cache")
	if img == nil {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_ = writePNG(w, img)
}

// jobEvents streams a job from the job registry. It serves render, template
// download, list resolve and card data download jobs
func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.Workspace.Jobs.Lookup(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	streamJob(w, r, j)
}

// streamJob writes a job's events as server-sent events, replaying the backlog
// to a late subscriber and ending after the terminal done event
func streamJob(w http.ResponseWriter, r *http.Request, j *jobs.Job) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	from := 0
	for {
		events, finished, wait := j.Stream(from)
		from += len(events)
		for _, e := range events {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		flusher.Flush()
		if finished {
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			return
		}
	}
}

// maxFontUpload bounds a font upload form, the font itself plus a megabyte of
// form framing
const maxFontUpload = fontdir.MaxFontBytes + 1<<20

// writePNG encodes img as the response body
func writePNG(w http.ResponseWriter, img image.Image) error { return png.Encode(w, img) }
