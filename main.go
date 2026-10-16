// Command pprof2ff converts Go pprof profiles and execution traces to Firefox Profiler format.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/google/pprof/profile"

	"github.com/vrutkovs/firefox-profiler-pprof/internal/convert"
	"github.com/vrutkovs/firefox-profiler-pprof/internal/ffprof"
	"github.com/vrutkovs/firefox-profiler-pprof/internal/serve"
)

const (
	openTimeout = 2 * time.Minute
	// manyEntries is roughly where the profiler UI gets sluggish.
	manyEntries = 1_000_000
)

// traceHeader matches Go execution trace files, e.g. "go 1.23 trace\x00\x00\x00".
var traceHeader = regexp.MustCompile(`^go 1\.\d+ trace\x00`)

const traceHeaderLen = 16

func main() {
	out := flag.String("o", "", "output file; .gz suffix gzips it (default stdout)")
	groupBy := flag.String("group-by", "", "pprof: split samples into threads by this label")
	minGo := flag.Duration("min-goroutine", time.Millisecond, "trace: hide goroutines running less than this without CPU samples")
	minState := flag.Duration("min-state", 0, "trace: drop goroutine state markers shorter than this, e.g. 100us")
	stacks := flag.String("stacks", "all", "trace: goroutine states turned into call-tree samples: all, blocked (off-CPU only) or off")
	open := flag.Bool("open", false, "open result in profiler.firefox.com")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: pprof2ff [flags] <profile|trace|->\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	mode, err := convert.ParseStackMode(*stacks)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pprof2ff:", err)
		os.Exit(2)
	}

	opts := options{
		pprof: convert.Options{GroupBy: *groupBy},
		trace: convert.TraceOptions{MinGoroutine: *minGo, MinState: *minState, Stacks: mode},
	}
	if err := run(flag.Arg(0), *out, opts, *open); err != nil {
		fmt.Fprintln(os.Stderr, "pprof2ff:", err)
		os.Exit(1)
	}
}

type options struct {
	pprof convert.Options
	trace convert.TraceOptions
}

func run(in, out string, opts options, open bool) error {
	f, err := openInput(in)
	if err != nil {
		return err
	}
	defer f.Close()

	p, err := convertInput(bufio.NewReader(f), opts)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}

	if out != "" || !open {
		if err := writeOutput(out, p); err != nil {
			return err
		}
	}
	if !open {
		return nil
	}

	var gz bytes.Buffer
	if err := ffprof.Write(&gz, p, true); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return serve.Open(ctx, gz.Bytes(), openTimeout)
}

// convertInput detects the input format by its header and converts it.
// Traces are streamed; pprof profiles are small and parsed whole.
func convertInput(r *bufio.Reader, opts options) (*ffprof.Profile, error) {
	head, _ := r.Peek(traceHeaderLen)
	if !traceHeader.Match(head) {
		pp, err := profile.Parse(r)
		if err != nil {
			return nil, err
		}
		return convert.Pprof(pp, opts.pprof)
	}

	if opts.pprof.GroupBy != "" {
		return nil, errors.New("-group-by applies to pprof profiles only")
	}
	p, st, err := convert.Trace(r, opts.trace)
	if err != nil {
		return nil, err
	}

	if st.ApproxRunning {
		fmt.Fprintln(os.Stderr, "pprof2ff: trace has no CPU samples; Running time is attributed to where goroutines stopped running (approximate). Fetch /debug/pprof/profile during the trace for real CPU stacks")
	}
	if st.Markers > manyEntries {
		fmt.Fprintf(os.Stderr, "pprof2ff: %d markers may be too many for the browser; try -min-state 100us\n", st.Markers)
	}
	if st.Samples > manyEntries {
		fmt.Fprintf(os.Stderr, "pprof2ff: %d samples may be too many for the browser; try -stacks blocked or off\n", st.Samples)
	}
	return p, nil
}

func openInput(in string) (io.ReadCloser, error) {
	if in == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(in)
}

func writeOutput(out string, p *ffprof.Profile) error {
	if out == "" {
		return ffprof.Write(os.Stdout, p, false)
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := ffprof.Write(f, p, strings.HasSuffix(out, ".gz")); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
