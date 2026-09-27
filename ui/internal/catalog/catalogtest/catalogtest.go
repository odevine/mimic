// Package catalogtest builds a minimal template bundle and serves a one-template
// catalog for it, so tests can drive downloads and the bundle cache offline
package catalogtest

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Bundle builds a minimal valid .mimic for normal 0.1.0 in memory and returns
// its bytes and SHA-256, mirroring the producer layout so the catalog download
// and the ZipAssetProvider both accept it
func Bundle(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, method uint16, body string) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatalf("adding %q: %v", name, err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("writing %q: %v", name, err)
		}
	}
	add("bundle.json", zip.Deflate, `{"format":1,"template":"normal","version":"0.1.0","minEngine":"0.3.0"}`)
	add("manifest.json", zip.Deflate, `{"template":"normal","width":744,"height":1039,"layers":[]}`)
	add("background/any.png", zip.Store, "fake-png-bytes")
	if err := zw.Close(); err != nil {
		t.Fatalf("closing bundle: %v", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// Server is a running catalog that lists Bundle as normal 0.1.0
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
	mux.HandleFunc("/normal.mimic", func(w http.ResponseWriter, r *http.Request) {
		s.downloads.Add(1)
		w.Write(bundle)
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"schema": 1,
			"templates": []map[string]any{{
				"name":   "normal",
				"latest": "0.1.0",
				"versions": []map[string]any{{
					"version":   "0.1.0",
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
	s.BundleURL = srv.URL + "/normal.mimic"
	return s
}
