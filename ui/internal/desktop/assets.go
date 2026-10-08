package desktop

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/odevine/mimic/ui/internal/apierr"
)

// Images is where the asset handler gets the pictures the page shows. The
// services implement it, so the page keeps using <img src> and the webview's
// cache, and no bytes cross the bridge
type Images interface {
	// RenderImage returns a finished render
	RenderImage(jobID string) (image.Image, string, error)
	// RunFile returns the path of one finished card of the latest run
	RunFile(runID string, n int) (string, error)
	// CardbackFile returns the path of the stored cardback
	CardbackFile() (string, error)
	// Symbol draws a mana pip. When known is the current tag the image is nil
	Symbol(code string, px int, known string) (image.Image, string, error)
}

// assetHandler serves /img/ from the services and everything else from the
// embedded frontend, and the launch check page when asked. The page's URLs are
// relative, so nothing opens a port
func assetHandler(img Images, frontend fs.FS, smoke bool) http.Handler {
	mux := http.NewServeMux()
	if smoke {
		// The launch check is the real page with its script added, so it runs in the
		// page itself, where the runtime delivers events
		mux.HandleFunc("GET /smoke.html", func(w http.ResponseWriter, r *http.Request) {
			index, err := fs.ReadFile(frontend, "index.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(withLaunchCheck(index))
		})
		mux.HandleFunc("GET /smoke.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Write(smokeJS)
		})
	}
	mux.HandleFunc("GET /img/render/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, _, err := img.RenderImage(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writePNG(w, m)
	})
	mux.HandleFunc("GET /img/run/{id}/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path, err := img.RunFile(r.PathValue("id"), n)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /img/cardback", func(w http.ResponseWriter, r *http.Request) {
		path, err := img.CardbackFile()
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /img/symbol", func(w http.ResponseWriter, r *http.Request) {
		px, _ := strconv.Atoi(r.URL.Query().Get("px"))
		m, tag, err := img.Symbol(r.URL.Query().Get("code"), px, r.Header.Get("If-None-Match"))
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "no-cache")
		if m == nil {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writePNG(w, m)
	})
	mux.Handle("/", http.FileServerFS(frontend))
	return mux
}

// statuses maps an error's kind to the HTTP status an image request gets
var statuses = map[apierr.Kind]int{
	apierr.Internal:   http.StatusInternalServerError,
	apierr.BadRequest: http.StatusBadRequest,
	apierr.NotFound:   http.StatusNotFound,
	apierr.Conflict:   http.StatusConflict,
	apierr.BadGateway: http.StatusBadGateway,
	apierr.TooLarge:   http.StatusRequestEntityTooLarge,
}

func fail(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), statuses[apierr.KindOf(err)])
}

// writePNG encodes m and writes it in one piece. The webview's response writer
// hands each write to C, and cgo refuses a slice that sits inside a Go struct
// holding pointers, which is where the PNG encoder keeps the small chunks it
// writes. A buffer's own bytes are safe
func writePNG(w http.ResponseWriter, m image.Image) {
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		http.Error(w, "encoding image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	w.Write(b.Bytes())
}

// withLaunchCheck adds the launch check's script to a page, before its closing
// body tag when it has one
func withLaunchCheck(index []byte) []byte {
	script := []byte(`<script type="module" src="/smoke.js"></script>`)
	if i := bytes.LastIndex(index, []byte("</body>")); i >= 0 {
		return append(append(append([]byte{}, index[:i]...), script...), index[i:]...)
	}
	return append(append([]byte{}, index...), script...)
}
