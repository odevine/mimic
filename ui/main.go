// Command mimic is a local front end for the mimic card renderer. It serves a
// small web app from a loopback HTTP server and opens it in the browser: search
// Scryfall with full query syntax, edit a match's fields, preview it through the
// normal template, and download the result. The whole binary is pure Go, so it
// builds with CGO_ENABLED=0 and cross-compiles to a single static file per target
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "address to bind, loopback with an ephemeral port by default")
	noOpen := flag.Bool("no-open", false, "do not open the browser at startup")
	flag.Parse()

	s := newServer()
	defer s.pipe.close()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("mimic: listening on %s: %v", *addr, err)
	}
	url := "http://" + ln.Addr().String()
	fmt.Println(url)

	srv := &http.Server{Handler: guard(s.mux)}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("mimic: serving: %v", err)
		}
	}()

	if !*noOpen {
		if err := openBrowser(url); err != nil {
			log.Printf("mimic: could not open browser, visit %s manually: %v", url, err)
		}
	}

	// Block until interrupted, then shut down gracefully so an in-flight request
	// finishes and the pipeline's providers close cleanly
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
