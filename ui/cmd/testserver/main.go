// Command testserver runs the app as a headless HTTP server for tests and for
// an agent that wants to look at the page. It builds the app with the server
// tag, starts a fake Scryfall, GitHub and template catalog, gives the app a
// scratch config folder, and prints the address to open. It never runs in a
// release and the binary it builds never ships
package main

import (
	"bytes"
	"context"
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
	"strconv"
	"time"

	"github.com/odevine/mimic/ui/internal/fakeupstream"
	"github.com/odevine/mimic/ui/internal/testbuild"
	"github.com/odevine/mimic/ui/testassets"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("testserver: %v", err)
	}
}

func run() error {
	port := flag.Int("port", 0, "port for the app, 0 for a free one")
	home := flag.String("home", "", "scratch config folder, a new temporary folder when empty")
	clean := flag.Bool("clean", false, "empty the scratch folder first")
	bin := flag.String("bin", "", "a server-tag build of the app to run, built when empty")
	stdin := flag.Bool("exit-on-stdin-close", false, "quit when standard input closes, for a parent that cannot signal this process through go run")
	logPath := flag.String("log", "", "file for the app's own output, standard error when empty")
	caps := flag.String("capabilities", os.Getenv(testbuild.EnvCapabilities), "feature keys to report live, separated by commas")
	flag.Parse()

	root, err := moduleRoot()
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
		exe = filepath.Join(dir, "mimic-server")
		build := exec.Command("go", "build", "-tags", "server", "-o", exe, ".")
		build.Dir = root
		// The linker's warnings are noise on a good build, so the output is shown
		// only when the build fails
		if out, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("building the server build: %w\n%s", err, out)
		}
	}

	if *port == 0 {
		if *port, err = freePort(); err != nil {
			return err
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
	app := exec.CommandContext(ctx, exe)
	app.Dir = root
	app.Stdout, app.Stderr = os.Stderr, os.Stderr
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			return err
		}
		defer f.Close()
		app.Stdout, app.Stderr = f, f
	}
	app.Env = append(os.Environ(),
		"WAILS_SERVER_HOST=127.0.0.1",
		"WAILS_SERVER_PORT="+strconv.Itoa(*port),
		testbuild.EnvHome+"="+scratch,
		testbuild.EnvUpstream+"="+fakeLn.Addr().String(),
		testbuild.EnvCapabilities+"="+*caps,
	)
	app.Cancel = func() error { return app.Process.Signal(os.Interrupt) }
	app.WaitDelay = 5 * time.Second
	if err := app.Start(); err != nil {
		return err
	}
	url := "http://127.0.0.1:" + strconv.Itoa(*port)
	if err := waitFor(ctx, url); err != nil {
		app.Process.Kill()
		return err
	}
	fmt.Printf("mimic test server listening at %s\nscratch folder: %s\n", url, scratch)

	err = app.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// moduleRoot finds the folder holding the ui module's go.mod, looking upward
// from the working directory
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if raw, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && containsModule(raw) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run testserver from inside the ui module")
		}
		dir = parent
	}
}

func containsModule(gomod []byte) bool {
	return bytes.HasPrefix(gomod, []byte("module github.com/odevine/mimic/ui\n")) || bytes.Contains(gomod, []byte("\nmodule github.com/odevine/mimic/ui\n"))
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
