package main

import (
	"net"
	"net/http"
	"strings"
)

// guard rejects requests that another website could make through the user's
// browser. A Host naming anything other than localhost or an IP literal is a DNS
// rebinding attempt, since a rebound name still carries the attacker's hostname.
// A state-changing request whose Origin is not this server's own came from
// another page. A request with no Origin at all is a script or curl, not a
// browser, and is let through so the API stays scriptable
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// allowedHost reports whether a Host header names this machine by localhost or
// by an IP literal, with or without a port
func allowedHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil
}
