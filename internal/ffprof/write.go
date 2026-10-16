package ffprof

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
)

// Write encodes p as JSON, gzip-compressed if gz is set.
// Threads are encoded one at a time: json.Encoder buffers its whole input,
// which for big traces would mean holding the whole output in memory.
func Write(w io.Writer, p *Profile, gz bool) error {
	bw := bufio.NewWriter(w)
	out := io.Writer(bw)

	var zw *gzip.Writer
	if gz {
		// Profiles can be hundreds of MB; default compression is ~4x slower for ~10% gain.
		zw, _ = gzip.NewWriterLevel(bw, gzip.BestSpeed)
		out = zw
	}

	fields := []struct {
		key string
		v   any
	}{
		{"meta", p.Meta},
		{"libs", p.Libs},
		{"pages", p.Pages},
		{"counters", p.Counters},
		{"shared", p.Shared},
	}

	ew := &errWriter{w: out}
	ew.str("{")
	for _, f := range fields {
		ew.str(`"` + f.key + `":`)
		ew.json(f.v)
		ew.str(",")
	}

	ew.str(`"threads":[`)
	for i, t := range p.Threads {
		if i > 0 {
			ew.str(",")
		}
		ew.json(t)
	}
	ew.str("]}\n")
	if ew.err != nil {
		return ew.err
	}

	if zw != nil {
		if err := zw.Close(); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// errWriter keeps the first error so writes can be chained.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) str(s string) {
	if e.err == nil {
		_, e.err = io.WriteString(e.w, s)
	}
}

func (e *errWriter) json(v any) {
	if e.err != nil {
		return
	}

	b, err := json.Marshal(v)
	if err != nil {
		e.err = err
		return
	}
	_, e.err = e.w.Write(b)
}
