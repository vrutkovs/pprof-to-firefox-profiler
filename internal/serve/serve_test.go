package serve

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	payload := []byte("\x1f\x8bdata")
	fetched := make(chan error, 1)

	browse = func(u string) error {
		src, err := url.QueryUnescape(strings.TrimPrefix(u, profilerOrigin+"/from-url/"))
		if err != nil {
			return err
		}

		go func() {
			resp, err := http.Get(src)
			if err != nil {
				fetched <- err
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != profilerOrigin {
				t.Errorf("CORS origin = %q", got)
			}
			if string(body) != string(payload) {
				t.Errorf("body = %q", body)
			}
			fetched <- nil
		}()
		return nil
	}

	if err := Open(context.Background(), payload, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-fetched; err != nil {
		t.Fatal(err)
	}
}

func TestOpenTimeout(t *testing.T) {
	browse = func(string) error { return nil }
	if err := Open(context.Background(), nil, 50*time.Millisecond); err == nil {
		t.Fatal("want timeout error")
	}
}
