// Package catalogtest builds a minimal template bundle and serves a one-template
// catalog for it, so tests can drive downloads and the bundle cache offline
package catalogtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/odevine/mimic/ui/internal/templatetest"
)

// ID is the id of the template the catalog lists
const ID = "test.frame"

// Bundle builds a valid .mimic for ID 0.1.0 and returns its bytes and SHA-256
func Bundle(t *testing.T) ([]byte, string) {
	t.Helper()
	return templatetest.Spec{ID: ID, Version: "0.1.0"}.Bundle(t)
}

// Server is a running catalog that lists Bundle as ID 0.1.0
type Server struct {
	// IndexURL is the catalog to point catalog.IndexURL at
	IndexURL string
	// BundleURL is where the listed version downloads from
	BundleURL string
	// SHA is the listed bundle's real SHA-256
	SHA string

	downloads atomic.Int32
}

// Downloads is how many times the bundle has been fetched
func (s *Server) Downloads() int { return int(s.downloads.Load()) }

// Serve starts a catalog server that is closed when the test ends. It encodes
// the index by hand so this package does not import catalog, whose own tests use it
func Serve(t *testing.T) *Server {
	t.Helper()
	bundle, sha := Bundle(t)
	s := &Server{SHA: sha}

	mux := http.NewServeMux()
	mux.HandleFunc("/"+ID+".mimic", func(w http.ResponseWriter, r *http.Request) {
		s.downloads.Add(1)
		w.Write(bundle)
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"schema": 1,
			"templates": []map[string]any{{
				"name":   ID,
				"latest": "0.1.0",
				"versions": []map[string]any{{
					"version":   "0.1.0",
					"format":    2,
					"minEngine": "0.3.0",
					"url":       s.BundleURL,
					"size":      len(bundle),
					"sha256":    sha,
				}},
			}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s.IndexURL = srv.URL + "/index.json"
	s.BundleURL = srv.URL + "/" + ID + ".mimic"
	return s
}
