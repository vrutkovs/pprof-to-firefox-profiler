package ffprof

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
)

// Write encodes p as JSON, gzip-compressed if gz is set.
func Write(w io.Writer, p *Profile, gz bool) error {
	bw := bufio.NewWriter(w)
	out := io.Writer(bw)

	var zw *gzip.Writer
	if gz {
		zw = gzip.NewWriter(bw)
		out = zw
	}

	if err := json.NewEncoder(out).Encode(p); err != nil {
		return err
	}
	if zw != nil {
		if err := zw.Close(); err != nil {
			return err
		}
	}
	return bw.Flush()
}
