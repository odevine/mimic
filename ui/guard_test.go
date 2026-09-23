package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := guard(ok)

	cases := []struct {
		name   string
		method string
		host   string
		origin string
		want   int
	}{
		{"loopback get", http.MethodGet, "127.0.0.1:8765", "", http.StatusOK},
		{"localhost get", http.MethodGet, "localhost:8765", "", http.StatusOK},
		{"ipv6 get", http.MethodGet, "[::1]:8765", "", http.StatusOK},
		{"lan ip get", http.MethodGet, "192.168.1.20:8765", "", http.StatusOK},
		{"rebound name", http.MethodGet, "evil.example:8765", "", http.StatusForbidden},
		{"localhost subdomain", http.MethodGet, "a.localhost:8765", "", http.StatusForbidden},
		{"same origin post", http.MethodPost, "127.0.0.1:8765", "http://127.0.0.1:8765", http.StatusOK},
		{"script post", http.MethodPost, "127.0.0.1:8765", "", http.StatusOK},
		{"cross origin post", http.MethodPost, "127.0.0.1:8765", "https://evil.example", http.StatusForbidden},
		{"other port post", http.MethodPut, "127.0.0.1:8765", "http://127.0.0.1:9999", http.StatusForbidden},
		{"null origin post", http.MethodPost, "127.0.0.1:8765", "null", http.StatusForbidden},
		{"cross origin get", http.MethodGet, "127.0.0.1:8765", "https://evil.example", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "/api/x", nil)
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}
