// Command pprof2ff converts Go pprof profiles to Firefox Profiler format.
package main

import (
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

const openTimeout = 2 * time.Minute

// traceHeader matches Go execution trace files, e.g. "go 1.23 trace\x00\x00\x00".
var traceHeader = regexp.MustCompile(`^go 1\.\d+ trace\x00`)

func main() {
	out := flag.String("o", "", "output file; .gz suffix gzips it (default stdout)")
	groupBy := flag.String("group-by", "", "split samples into threads by this pprof label")
	open := flag.Bool("open", false, "open result in profiler.firefox.com")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: pprof2ff [flags] <profile|->\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(flag.Arg(0), *out, *groupBy, *open); err != nil {
		fmt.Fprintln(os.Stderr, "pprof2ff:", err)
		os.Exit(1)
	}
}

func run(in, out, groupBy string, open bool) error {
	data, err := readInput(in)
	if err != nil {
		return err
	}
	if traceHeader.Match(data) {
		return errors.New("go execution traces are not supported yet")
	}

	pp, err := profile.ParseData(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", in, err)
	}

	p, err := convert.Pprof(pp, convert.Options{GroupBy: groupBy})
	if err != nil {
		return err
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

func readInput(in string) ([]byte, error) {
	if in == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(in)
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
