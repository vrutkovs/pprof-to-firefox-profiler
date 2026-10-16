// Package serve hands a profile to profiler.firefox.com via a one-shot local server.
package serve

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	profilerOrigin = "https://profiler.firefox.com"
	fileName       = "/profile.json.gz"
	shutdownWait   = 5 * time.Second
)

// Open serves gzipped profile data on loopback, opens the profiler on it and
// returns after the first successful download, ctx cancellation or timeout.
func Open(ctx context.Context, gz []byte, timeout time.Duration) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+fileName, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", profilerOrigin)
		w.Header().Set("Content-Type", "application/gzip")
		if _, err := w.Write(gz); err != nil {
			return
		}
		select {
		case done <- struct{}{}:
		default:
		}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	src := "http://" + ln.Addr().String() + fileName
	target := profilerOrigin + "/from-url/" + url.QueryEscape(src)
	fmt.Fprintln(os.Stderr, "opening", target)
	if err := browse(target); err != nil {
		fmt.Fprintln(os.Stderr, "cannot open browser:", err, "- open the URL above manually")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("profiler did not fetch the profile within %s", timeout)
		}
		return ctx.Err()
	}
}

// browse opens u in the default browser; replaced in tests.
var browse = func(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
