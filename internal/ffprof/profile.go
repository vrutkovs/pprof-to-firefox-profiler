// Package ffprof models the Firefox Profiler processed profile format.
//
// Schema: https://github.com/firefox-devtools/profiler/blob/main/src/types/profile.ts
package ffprof

import (
	"cmp"
	"slices"
)

const (
	GeckoVersion     = 37
	ProcessedVersion = 75
)

// Frame flags (FrameFlag in profile.ts).
const (
	FrameIsInlined   = 1 << 0
	FrameHasAddress  = 1 << 1
	FrameHasCategory = 1 << 2
	FrameHasLine     = 1 << 4
)

// Func flags (FuncFlag in profile.ts).
const (
	FuncHasResource = 1 << 2
	FuncHasSource   = 1 << 3
	FuncHasLine     = 1 << 4
)

// Weight types (WeightType in profile.ts).
const (
	WeightSamples   = "samples"
	WeightTracingMs = "tracing-ms"
	WeightBytes     = "bytes"
)

const resourceTypeUnknown = 0

type Profile struct {
	Meta     Meta       `json:"meta"`
	Libs     []Lib      `json:"libs"`
	Pages    []any      `json:"pages"`
	Counters []*Counter `json:"counters"`
	Shared   Shared     `json:"shared"`
	Threads  []*Thread  `json:"threads"`
}

type Meta struct {
	Interval                   float64        `json:"interval"`
	StartTime                  float64        `json:"startTime"`
	ProfilingStartTime         *float64       `json:"profilingStartTime,omitempty"`
	ProfilingEndTime           *float64       `json:"profilingEndTime,omitempty"`
	ProcessType                int            `json:"processType"`
	Categories                 []Category     `json:"categories"`
	Product                    string         `json:"product"`
	Stackwalk                  int            `json:"stackwalk"`
	Version                    int            `json:"version"`
	PreprocessedProfileVersion int            `json:"preprocessedProfileVersion"`
	MarkerSchema               []MarkerSchema `json:"markerSchema"`
	Symbolicated               bool           `json:"symbolicated"`
	SymbolicationNotSupported  bool           `json:"symbolicationNotSupported"`
	UsesOnlyOneStackType       bool           `json:"usesOnlyOneStackType"`
	SourceCodeIsNotOnSearchfox bool           `json:"sourceCodeIsNotOnSearchfox"`
	KeepProfileThreadOrder     bool           `json:"keepProfileThreadOrder"`
	ImportedFrom               string         `json:"importedFrom,omitempty"`
	Extra                      []ExtraSection `json:"extra,omitempty"`
	InitialVisibleThreads      []int          `json:"initialVisibleThreads,omitempty"`
	InitialSelectedThreads     []int          `json:"initialSelectedThreads,omitempty"`
}

type Category struct {
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	Subcategories []string `json:"subcategories"`
}

type ExtraSection struct {
	Label   string       `json:"label"`
	Entries []ExtraEntry `json:"entries"`
}

type ExtraEntry struct {
	Label  string `json:"label"`
	Format string `json:"format"`
	Value  any    `json:"value"`
}

type Lib struct {
	Arch       string  `json:"arch"`
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	DebugName  string  `json:"debugName"`
	DebugPath  string  `json:"debugPath"`
	BreakpadID string  `json:"breakpadId"`
	CodeID     *string `json:"codeId"`
}

type Shared struct {
	StackTable          StackTable          `json:"stackTable"`
	FrameTable          FrameTable          `json:"frameTable"`
	FuncTable           FuncTable           `json:"funcTable"`
	ResourceTable       ResourceTable       `json:"resourceTable"`
	NativeSymbols       NativeSymbolTable   `json:"nativeSymbols"`
	StringArray         []string            `json:"stringArray"`
	Sources             SourceTable         `json:"sources"`
	SourceLocationTable SourceLocationTable `json:"sourceLocationTable"`
}

type StackTable struct {
	Frame        []int32 `json:"frame"`
	PrefixOffset []int32 `json:"prefixOffset"`
	Length       int     `json:"length"`
}

type FrameTable struct {
	Flags            []int32   `json:"flags"`
	Address          []uint32  `json:"address"`
	Lib              []int32   `json:"lib"`
	Category         []int32   `json:"category"`
	Subcategory      []int32   `json:"subcategory"`
	Func             []int32   `json:"func"`
	NativeSymbol     []int32   `json:"nativeSymbol"`
	InnerWindowID    []float64 `json:"innerWindowID"`
	Line             []int32   `json:"line"`
	Column           []int32   `json:"column"`
	OriginalLocation []int32   `json:"originalLocation"`
	Length           int       `json:"length"`
}

type FuncTable struct {
	Flags            []int32 `json:"flags"`
	Name             []int32 `json:"name"`
	Resource         []int32 `json:"resource"`
	Source           []int32 `json:"source"`
	LineNumber       []int32 `json:"lineNumber"`
	ColumnNumber     []int32 `json:"columnNumber"`
	OriginalLocation []int32 `json:"originalLocation"`
	Length           int     `json:"length"`
}

type ResourceTable struct {
	Name   []int32  `json:"name"`
	Host   []*int32 `json:"host"`
	Type   []int    `json:"type"`
	Length int      `json:"length"`
}

type NativeSymbolTable struct {
	LibIndex     []int32  `json:"libIndex"`
	Address      []uint32 `json:"address"`
	Name         []int32  `json:"name"`
	FunctionSize []int32  `json:"functionSize"`
	Length       int      `json:"length"`
}

type SourceTable struct {
	ID           []*string `json:"id"`
	Filename     []int32   `json:"filename"`
	StartLine    []int32   `json:"startLine"`
	StartColumn  []int32   `json:"startColumn"`
	SourceMapURL []*int32  `json:"sourceMapURL"`
	Content      []*string `json:"content"`
	Length       int       `json:"length"`
}

type SourceLocationTable struct {
	Source []int32 `json:"source"`
	Line   []int32 `json:"line"`
	Column []int32 `json:"column"`
	Length int     `json:"length"`
}

type Thread struct {
	ProcessType           string   `json:"processType"`
	ProcessStartupTime    float64  `json:"processStartupTime"`
	ProcessShutdownTime   *float64 `json:"processShutdownTime"`
	RegisterTime          float64  `json:"registerTime"`
	UnregisterTime        *float64 `json:"unregisterTime"`
	PausedRanges          []any    `json:"pausedRanges"`
	ShowMarkersInTimeline bool     `json:"showMarkersInTimeline,omitempty"`
	Name                  string   `json:"name"`
	ProcessName           string   `json:"processName,omitempty"`
	IsMainThread          bool     `json:"isMainThread"`
	Pid                   string   `json:"pid"`
	Tid                   int      `json:"tid"`
	Samples               Samples  `json:"samples"`
	Markers               Markers  `json:"markers"`
}

// Marker phases (MarkerPhase in profile.ts).
const (
	PhaseInstant  = 0
	PhaseInterval = 1
)

// Add appends one marker. For instant markers end is ignored.
func (m *Markers) Add(name int32, start, end float64, phase, category int32, data any) {
	m.Name = append(m.Name, name)
	m.StartTime = append(m.StartTime, start)
	m.EndTime = append(m.EndTime, end)
	m.Phase = append(m.Phase, phase)
	m.Category = append(m.Category, category)
	m.Data = append(m.Data, data)
	m.Length++
}

// Cause is a marker backtrace (CauseBacktrace in markers.ts).
type Cause struct {
	Tid   int     `json:"tid"`
	Time  float64 `json:"time"`
	Stack int32   `json:"stack"`
}

type MarkerSchema struct {
	Name         string        `json:"name"`
	TooltipLabel string        `json:"tooltipLabel,omitempty"`
	TableLabel   string        `json:"tableLabel,omitempty"`
	ChartLabel   string        `json:"chartLabel,omitempty"`
	Display      []string      `json:"display"`
	Fields       []MarkerField `json:"fields"`
	Description  string        `json:"description,omitempty"`
	IsStackBased bool          `json:"isStackBased,omitempty"`
}

type MarkerField struct {
	Key    string `json:"key"`
	Label  string `json:"label,omitempty"`
	Format string `json:"format"`
	Hidden bool   `json:"hidden,omitempty"`
}

// Counter is a RawCounter. Count holds deltas; the graph shows their running sum.
type Counter struct {
	Name            string         `json:"name"`
	Category        string         `json:"category"`
	Description     string         `json:"description"`
	Pid             string         `json:"pid"`
	MainThreadIndex int            `json:"mainThreadIndex"`
	Samples         CounterSamples `json:"samples"`
	Display         CounterDisplay `json:"display"`
}

type CounterSamples struct {
	Time   []float64 `json:"time"`
	Count  []float64 `json:"count"`
	Length int       `json:"length"`
}

// Add appends a delta sample.
func (s *CounterSamples) Add(time, delta float64) {
	s.Time = append(s.Time, time)
	s.Count = append(s.Count, delta)
	s.Length++
}

type CounterDisplay struct {
	GraphType            string       `json:"graphType"`
	Unit                 string       `json:"unit"`
	Color                string       `json:"color"`
	MarkerSchemaLocation *string      `json:"markerSchemaLocation"`
	SortWeight           int          `json:"sortWeight"`
	Label                string       `json:"label"`
	TooltipRows          []TooltipRow `json:"tooltipRows"`
}

type TooltipRow struct {
	Type   string        `json:"type"`
	Source string        `json:"source"`
	Format TooltipFormat `json:"format"`
	Label  string        `json:"label"`
}

type TooltipFormat struct {
	Unit string `json:"unit"`
}

type Samples struct {
	Stack      []int32   `json:"stack"`
	Time       []float64 `json:"time"`
	Weight     []float64 `json:"weight"`
	WeightType string    `json:"weightType"`
	Length     int       `json:"length"`
}

// Add appends one sample.
func (s *Samples) Add(stack int32, time, weight float64) {
	s.Stack = append(s.Stack, stack)
	s.Time = append(s.Time, time)
	s.Weight = append(s.Weight, weight)
	s.Length++
}

// SortByTime stably orders samples by time.
func (s *Samples) SortByTime() {
	if slices.IsSorted(s.Time) {
		return
	}

	idx := make([]int, s.Length)
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(s.Time[a], s.Time[b]) })

	stack, time, weight := make([]int32, s.Length), make([]float64, s.Length), make([]float64, s.Length)
	for i, j := range idx {
		stack[i], time[i], weight[i] = s.Stack[j], s.Time[j], s.Weight[j]
	}
	s.Stack, s.Time, s.Weight = stack, time, weight
}

type Markers struct {
	Data      []any     `json:"data"`
	Name      []int32   `json:"name"`
	StartTime []float64 `json:"startTime"`
	EndTime   []float64 `json:"endTime"`
	Phase     []int32   `json:"phase"`
	Category  []int32   `json:"category"`
	Length    int       `json:"length"`
}

// NewThread returns a thread with all columns initialized, so they encode as [] not null.
func NewThread(name, process, pid string, tid int, weightType string) *Thread {
	return &Thread{
		ProcessType:  "default",
		PausedRanges: []any{},
		Name:         name,
		ProcessName:  process,
		Pid:          pid,
		Tid:          tid,
		Samples: Samples{
			Stack:      []int32{},
			Time:       []float64{},
			Weight:     []float64{},
			WeightType: weightType,
		},
		Markers: Markers{
			Data:      []any{},
			Name:      []int32{},
			StartTime: []float64{},
			EndTime:   []float64{},
			Phase:     []int32{},
			Category:  []int32{},
		},
	}
}
