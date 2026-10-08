// Package testbuild holds the seams that let a build made with the server or mcp
// tag run safely beside a developer's own copy of the app and without the
// internet. The binary calls Apply first thing in such a build, and the tags are
// what keep it out of a release. It imports no Wails, so it builds without cgo
package testbuild

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// The environment variables a test build reads
const (
	// EnvHome names the scratch folder standing in for the user config folder.
	// It is required, so a test build never touches real settings
	EnvHome = "MIMIC_E2E_HOME"
	// EnvUpstream is the host:port that answers every outbound request. Left
	// empty, every request is refused
	EnvUpstream = "MIMIC_E2E_UPSTREAM"
	// EnvCapabilities lists feature keys, separated by commas, to report live
	EnvCapabilities = "MIMIC_E2E_CAPABILITIES"
)

// refused is an address nothing listens on, so a test build with no upstream
// fails every network call at once instead of reaching the internet
const refused = "127.0.0.1:1"

// Apply configures the process from env, which is os.Getenv in the binary. It
// runs before the workspace is built
func Apply(env func(string) string) error {
	home := env(EnvHome)
	if home == "" {
		return errors.New(EnvHome + " must name a scratch folder, since a test build does not use the real config folder")
	}
	keys := splitList(env(EnvCapabilities))
	if err := settings.ForceLive(keys...); err != nil {
		return err
	}
	workspace.UserConfigDir = func() (string, error) { return home, nil }
	// A checkout with loose template assets would render from them, and a test
	// should see the same template on every machine
	pipeline.LooseDirBases = nil

	upstream := env(EnvUpstream)
	if upstream == "" {
		upstream = refused
	}
	Redirect(upstream)
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// Redirect makes every outbound HTTP and HTTPS request go to addr, as plain
// HTTP with its original Host header, so one fake server can stand in for
// Scryfall, GitHub and the template catalog. The paced Scryfall client clones
// the default transport when the workspace is built, so this runs before that.
// It returns a function that puts the transport back
func Redirect(addr string) (restore func()) {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic(fmt.Sprintf("testbuild: the default transport is a %T", http.DefaultTransport))
	}
	saved := *t.Clone()
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	t.DialContext = dial
	// A connection returned from DialTLSContext is taken as already secured, so
	// an https request reaches the fake as plain HTTP
	t.DialTLSContext = dial
	t.Proxy = nil
	t.ForceAttemptHTTP2 = false
	t.CloseIdleConnections()
	return func() {
		t.DialContext, t.DialTLSContext, t.Proxy, t.ForceAttemptHTTP2 = saved.DialContext, saved.DialTLSContext, saved.Proxy, saved.ForceAttemptHTTP2
		t.CloseIdleConnections()
	}
}
