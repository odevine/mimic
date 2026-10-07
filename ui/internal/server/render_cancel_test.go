package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRenderCancelStopsJob(t *testing.T) {
	s := runServer(t)
	id, j := s.newJob()
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/render/"+id+"/cancel", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d, want 204", rec.Code)
	}
	if ctx.Err() == nil {
		t.Fatal("job context still live after cancel")
	}

	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/render/nope/cancel", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown job status = %d, want 404", rec.Code)
	}
}
