// Command testserver runs a test build of the app beside a fake Scryfall, GitHub
// and template catalog, with a scratch config folder, for tests and for an agent
// that wants to look at the app. In the default server mode the app is a
// headless HTTP server whose address is printed. In mcp mode it is the real
// window with an MCP endpoint on loopback, and the endpoint is written to
// bin/mcp.json for the mcpcall command. It builds the app itself and the binary
// never ships
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/fakeupstream"
	"github.com/odevine/mimic/ui/internal/mcpstate"
	"github.com/odevine/mimic/ui/internal/testbuild"
	"github.com/odevine/mimic/ui/testassets"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("testserver: %v", err)
	}
}

func run() error {
	mode := flag.String("mode", "server", "server for a headless HTTP server, mcp for the real window with an MCP endpoint")
	port := flag.Int("port", 0, "port for the app in server mode, 0 for a free one")
	home := flag.String("home", "", "scratch config folder, a new temporary folder when empty")
	clean := flag.Bool("clean", false, "empty the scratch folder first")
	bin := flag.String("bin", "", "a build of the app to run, built when empty")
	stdin := flag.Bool("exit-on-stdin-close", false, "quit when standard input closes, for a parent that cannot signal this process through go run")
	logPath := flag.String("log", "", "file for the app's own output, standard error when empty")
	caps := flag.String("capabilities", os.Getenv(testbuild.EnvCapabilities), "feature keys to report live, separated by commas")
	flag.Parse()
	if *mode != "server" && *mode != "mcp" {
		return fmt.Errorf("-mode is %q, want server or mcp", *mode)
	}

	root, err := mcpstate.ModuleRoot()
	if err != nil {
		return err
	}
	scratch := *home
	if scratch == "" {
		if scratch, err = os.MkdirTemp("", "mimic-testserver-"); err != nil {
			return err
		}
	}
	if scratch, err = filepath.Abs(scratch); err != nil {
		return err
	}
	if *clean {
		if err := os.RemoveAll(scratch); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return err
	}

	cards, err := fs.Sub(testassets.Scryfall, "scryfall")
	if err != nil {
		return err
	}
	fake, err := fakeupstream.New(cards)
	if err != nil {
		return err
	}
	fakeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	go http.Serve(fakeLn, fake)

	exe := *bin
	if exe == "" {
		dir, err := os.MkdirTemp("", "mimic-testserver-bin-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		exe = filepath.Join(dir, "mimic-test")
		tags := *mode
		if *mode == "mcp" && runtime.GOOS == "linux" {
			tags += ",gtk3"
		}
		build := exec.Command("go", "build", "-tags", tags, "-o", exe, ".")
		build.Dir = root
		// The linker's warnings are noise on a good build, so the output is shown
		// only when the build fails
		if out, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("building the %s build: %w\n%s", *mode, err, out)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *stdin {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}

	env := append(os.Environ(),
		testbuild.EnvHome+"="+scratch,
		testbuild.EnvUpstream+"="+fakeLn.Addr().String(),
		testbuild.EnvCapabilities+"="+*caps,
	)
	var token string
	var url string
	if *mode == "server" {
		if *port == 0 {
			if *port, err = freePort(); err != nil {
				return err
			}
		}
		url = "http://127.0.0.1:" + strconv.Itoa(*port)
		env = append(env, "WAILS_SERVER_HOST=127.0.0.1", "WAILS_SERVER_PORT="+strconv.Itoa(*port))
	} else {
		if token, err = randomToken(); err != nil {
			return err
		}
		env = append(env, "WAILS_MCP_HOST=127.0.0.1", "WAILS_MCP_PORT=0", "WAILS_MCP_TOKEN="+token)
	}

	// The app's output goes to the terminal or a file, and in mcp mode a watcher
	// reads the endpoint out of it
	var out io.Writer = os.Stderr
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	found := make(chan string, 1)
	watch := &lineWatcher{re: mcpURL, found: found}
	app := exec.CommandContext(ctx, exe)
	app.Dir = root
	app.Stdout, app.Stderr = io.MultiWriter(out, watch), io.MultiWriter(out, watch)
	app.Env = env
	app.Cancel = func() error { return interrupt(app.Process) }
	app.WaitDelay = 5 * time.Second
	if err := app.Start(); err != nil {
		return err
	}

	exited := make(chan error, 1)
	go func() { exited <- app.Wait() }()

	if *mode == "mcp" {
		select {
		case url = <-found:
		case err := <-exited:
			return fmt.Errorf("the app exited before its MCP endpoint was ready: %v", err)
		case <-time.After(90 * time.Second):
			app.Process.Kill()
			return errors.New("the app did not report an MCP endpoint within 90 seconds")
		}
		state := mcpstate.State{URL: url, Token: token, PID: os.Getpid(), Home: scratch}
		if err := mcpstate.Write(root, state); err != nil {
			app.Process.Kill()
			return err
		}
		defer mcpstate.Remove(root, state.PID)
		fmt.Printf("mimic mcp test build ready at %s\nscratch folder: %s\nstate file: %s\n", url, scratch, mcpstate.Path(root))
	} else {
		if err := waitFor(ctx, url); err != nil {
			app.Process.Kill()
			return err
		}
		fmt.Printf("mimic test server listening at %s\nscratch folder: %s\n", url, scratch)
	}

	select {
	case err := <-exited:
		if ctx.Err() != nil {
			return nil
		}
		return err
	case <-ctx.Done():
		<-exited
		return nil
	}
}

// mcpURL matches the line Wails logs when its MCP server starts
var mcpURL = regexp.MustCompile(`url=(http://\S+/mcp)`)

// lineWatcher passes the first match of re in what is written to found
type lineWatcher struct {
	re    *regexp.Regexp
	found chan string
	mu    sync.Mutex
	buf   strings.Builder
	done  bool
}

func (w *lineWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return len(p), nil
	}
	w.buf.Write(p)
	text := w.buf.String()
	if m := w.re.FindStringSubmatch(text); m != nil {
		w.done = true
		w.found <- m[1]
		return len(p), nil
	}
	// Keep only the unfinished last line, so the buffer stays small
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		w.buf.Reset()
		w.buf.WriteString(text[i+1:])
	}
	return len(p), nil
}

// interrupt asks a process to stop, which Windows cannot do with a signal
func interrupt(p *os.Process) error {
	if runtime.GOOS == "windows" {
		return p.Kill()
	}
	return p.Signal(os.Interrupt)
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitFor polls url until the app answers or the context ends
func waitFor(ctx context.Context, url string) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if resp, err := http.Get(url + "/"); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the app did not answer within 60 seconds")
}
