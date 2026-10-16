// Package convert turns Go profiling data into Firefox Profiler profiles.
package convert

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/pprof/profile"

	"github.com/vrutkovs/firefox-profiler-pprof/internal/ffprof"
)

const (
	noLabel   = "<none>"
	nsPerMs   = float64(time.Millisecond)
	defaultMs = 1
)

// Options tune the conversion.
type Options struct {
	// GroupBy splits samples into threads by this pprof label key.
	GroupBy string
}

// Pprof converts p. Each sample type becomes a process; GroupBy values become its threads.
func Pprof(p *profile.Profile, opt Options) (*ffprof.Profile, error) {
	if len(p.SampleType) == 0 {
		return nil, fmt.Errorf("profile has no sample types")
	}

	interval := pprofInterval(p)
	b := ffprof.NewBuilder(pprofMeta(p, interval))
	c := &pprofConv{b: b, libs: map[uint64]int32{}, locs: map[uint64][]int32{}}
	groups := groupKeys(p, opt.GroupBy)
	def := defaultType(p)

	tid := 0
	for ti, st := range p.SampleType {
		wt, scale := weightUnit(st.Unit)
		proc := st.Type + " (" + st.Unit + ")"

		// One thread per group value, in sorted order.
		threads := map[string]*ffprof.Thread{}
		for gi, g := range groups {
			name := proc
			if opt.GroupBy != "" {
				name = opt.GroupBy + "=" + g
			}

			t := ffprof.NewThread(name, proc, st.Type, tid, wt)
			t.IsMainThread = gi == 0
			idx := b.AddThread(t)
			threads[g] = t
			tid++

			b.Profile().Meta.InitialVisibleThreads = append(b.Profile().Meta.InitialVisibleThreads, idx)
			if ti == def && gi == 0 {
				b.Profile().Meta.InitialSelectedThreads = []int{idx}
			}
		}

		for _, s := range p.Sample {
			v := s.Value[ti]
			if v == 0 {
				continue
			}

			stack := c.stack(s.Location)
			if stack == ffprof.NoStack {
				continue
			}

			t := threads[groupOf(s, opt.GroupBy)]
			t.Samples.Add(stack, float64(t.Samples.Length)*interval, float64(v)*scale)
		}
	}

	return b.Profile(), nil
}

type pprofConv struct {
	b    *ffprof.Builder
	libs map[uint64]int32   // mapping ID -> lib
	locs map[uint64][]int32 // location ID -> frames, outermost first
}

// stack interns locations (leaf first in pprof) root to leaf.
func (c *pprofConv) stack(locs []*profile.Location) int32 {
	st := int32(ffprof.NoStack)
	for _, l := range slices.Backward(locs) {
		for _, f := range c.frames(l) {
			st = c.b.Stack(st, f)
		}
	}
	return st
}

// frames expands a location into frames. pprof lists inlined
// functions first (Line[0] is innermost), so walk it backwards.
func (c *pprofConv) frames(l *profile.Location) []int32 {
	if fr, ok := c.locs[l.ID]; ok {
		return fr
	}

	lib, addr := c.address(l)
	var fr []int32

	if len(l.Line) == 0 {
		res := ""
		if l.Mapping != nil {
			res = filepath.Base(l.Mapping.File)
		}
		fn := c.b.Func(ffprof.Func{Name: fmt.Sprintf("0x%x", l.Address), Resource: res})
		fr = append(fr, c.b.Frame(ffprof.Frame{Func: fn, Category: catOther, Lib: lib, Address: addr}))
	}

	for i, ln := range slices.Backward(l.Line) {
		name, file, start := "?", "", int64(0)
		if ln.Function != nil {
			name, file, start = ln.Function.Name, ln.Function.Filename, ln.Function.StartLine
		}

		pkg := goPackage(name)
		fn := c.b.Func(ffprof.Func{Name: name, File: file, Line: int32(start), Resource: pkg})
		fr = append(fr, c.b.Frame(ffprof.Frame{
			Func:     fn,
			Line:     int32(ln.Line),
			Category: classify(name, pkg),
			Inlined:  i != len(l.Line)-1,
			Lib:      lib,
			Address:  addr,
		}))
	}

	c.locs[l.ID] = fr
	return fr
}

// address returns lib and lib-relative address, or NoLib when unknown or > 4GiB.
func (c *pprofConv) address(l *profile.Location) (int32, uint32) {
	m := l.Mapping
	if m == nil || l.Address == 0 || l.Address < m.Start {
		return ffprof.NoLib, 0
	}

	rel := l.Address - m.Start + m.Offset
	if rel > math.MaxUint32 {
		return ffprof.NoLib, 0
	}

	lib, ok := c.libs[m.ID]
	if !ok {
		name := filepath.Base(m.File)
		lib = c.b.Lib(ffprof.Lib{
			Name: name, Path: m.File, DebugName: name, DebugPath: m.File, BreakpadID: m.BuildID,
		})
		c.libs[m.ID] = lib
	}
	return lib, uint32(rel)
}

// weightUnit maps a pprof unit to a profiler weight type and value scale.
func weightUnit(unit string) (string, float64) {
	switch unit {
	case "nanoseconds":
		return ffprof.WeightTracingMs, 1e-6
	case "microseconds":
		return ffprof.WeightTracingMs, 1e-3
	case "milliseconds":
		return ffprof.WeightTracingMs, 1
	case "seconds":
		return ffprof.WeightTracingMs, 1e3
	case "bytes":
		return ffprof.WeightBytes, 1
	}
	return ffprof.WeightSamples, 1
}

// pprofInterval is the sampling period in ms for time-based profiles, else 1.
func pprofInterval(p *profile.Profile) float64 {
	if p.PeriodType == nil || p.Period <= 0 {
		return defaultMs
	}

	wt, scale := weightUnit(p.PeriodType.Unit)
	if wt != ffprof.WeightTracingMs {
		return defaultMs
	}
	return float64(p.Period) * scale
}

// defaultType picks the sample type to select initially; pprof defaults to the last one.
func defaultType(p *profile.Profile) int {
	for i, st := range p.SampleType {
		if st.Type == p.DefaultSampleType {
			return i
		}
	}
	return len(p.SampleType) - 1
}

func groupOf(s *profile.Sample, key string) string {
	if key == "" {
		return ""
	}
	if v := s.Label[key]; len(v) > 0 {
		return v[0]
	}
	if v := s.NumLabel[key]; len(v) > 0 {
		return strconv.FormatInt(v[0], 10)
	}
	return noLabel
}

// groupKeys returns sorted distinct group values, with noLabel last.
func groupKeys(p *profile.Profile, key string) []string {
	seen := map[string]bool{}
	for _, s := range p.Sample {
		seen[groupOf(s, key)] = true
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		if k != noLabel {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)

	if seen[noLabel] {
		keys = append(keys, noLabel)
	}
	if len(keys) == 0 {
		keys = append(keys, "")
	}
	return keys
}

func pprofMeta(p *profile.Profile, interval float64) ffprof.Meta {
	product := "pprof"
	if len(p.Mapping) > 0 && p.Mapping[0].File != "" {
		product += ": " + filepath.Base(p.Mapping[0].File)
	}

	var info []ffprof.ExtraEntry
	add := func(label, value string) {
		if value != "" {
			info = append(info, ffprof.ExtraEntry{Label: label, Format: "string", Value: value})
		}
	}
	if p.DurationNanos > 0 {
		add("Duration", time.Duration(p.DurationNanos).String())
	}
	if p.PeriodType != nil {
		add("Period", fmt.Sprintf("%d %s/%s", p.Period, p.PeriodType.Unit, p.PeriodType.Type))
	}
	add("Comments", strings.Join(p.Comments, "\n"))
	add("Drop frames", p.DropFrames)
	add("Keep frames", p.KeepFrames)

	m := ffprof.Meta{
		Interval:                   interval,
		StartTime:                  float64(p.TimeNanos) / nsPerMs,
		Categories:                 categories,
		Product:                    product,
		Symbolicated:               true,
		SymbolicationNotSupported:  true,
		UsesOnlyOneStackType:       true,
		SourceCodeIsNotOnSearchfox: true,
		KeepProfileThreadOrder:     true,
		ImportedFrom:               "pprof",
	}
	if len(info) > 0 {
		m.Extra = []ffprof.ExtraSection{{Label: "pprof", Entries: info}}
	}
	return m
}
