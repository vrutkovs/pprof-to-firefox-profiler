package convert

import (
	"bytes"
	"crypto/sha256"
	"runtime"
	"runtime/pprof"
	"slices"
	"testing"
	"time"

	"github.com/google/pprof/profile"

	"github.com/vrutkovs/firefox-profiler-pprof/internal/ffprof"
)

// testProfile: main -> work (inlines helper) -> runtime.mallocgc, plus recursion and labels.
func testProfile() *profile.Profile {
	m := &profile.Mapping{ID: 1, Start: 0x1000, Limit: 0x9000, File: "/bin/app", BuildID: "abc"}
	fn := func(id uint64, name string) *profile.Function {
		return &profile.Function{ID: id, Name: name, Filename: "/src/" + name + ".go", StartLine: 10}
	}
	fMain, fWork, fHelper, fMalloc, fRec := fn(1, "main.main"), fn(2, "main.work"), fn(3, "main.helper"),
		fn(4, "runtime.mallocgc"), fn(5, "github.com/x/y.(*T).rec[...]")

	lMain := &profile.Location{ID: 1, Mapping: m, Address: 0x1100, Line: []profile.Line{{Function: fMain, Line: 11}}}
	// Inlined: helper is innermost (Line[0]), work is the physical frame.
	lWork := &profile.Location{ID: 2, Mapping: m, Address: 0x1200, Line: []profile.Line{
		{Function: fHelper, Line: 21}, {Function: fWork, Line: 31},
	}}
	lMalloc := &profile.Location{ID: 3, Mapping: m, Address: 0x1300, Line: []profile.Line{{Function: fMalloc, Line: 41}}}
	lRec := &profile.Location{ID: 4, Mapping: m, Address: 0x1400, Line: []profile.Line{{Function: fRec, Line: 51}}}

	return &profile.Profile{
		SampleType: []*profile.ValueType{{Type: "samples", Unit: "count"}, {Type: "cpu", Unit: "nanoseconds"}},
		PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:     10_000_000,
		TimeNanos:  1_700_000_000_000_000_000,
		Sample: []*profile.Sample{
			{Location: []*profile.Location{lMalloc, lWork, lMain}, Value: []int64{2, 20_000_000}, Label: map[string][]string{"job": {"b"}}},
			{Location: []*profile.Location{lWork, lMain}, Value: []int64{1, 10_000_000}, Label: map[string][]string{"job": {"a"}}},
			{Location: []*profile.Location{lRec, lRec, lMain}, Value: []int64{3, 30_000_000}},
			{Location: []*profile.Location{lMain}, Value: []int64{0, 0}},
		},
		Location: []*profile.Location{lMain, lWork, lMalloc, lRec},
		Function: []*profile.Function{fMain, fWork, fHelper, fMalloc, fRec},
		Mapping:  []*profile.Mapping{m},
	}
}

func mustConvert(t *testing.T, p *profile.Profile, opt Options) *ffprof.Profile {
	t.Helper()
	out, err := Pprof(p, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := ffprof.Validate(out); err != nil {
		t.Fatal(err)
	}
	return out
}

// stackNames returns func names root to leaf.
func stackNames(p *ffprof.Profile, st int32) []string {
	s := &p.Shared
	var names []string
	for {
		f := s.StackTable.Frame[st]
		names = append(names, s.StringArray[s.FuncTable.Name[s.FrameTable.Func[f]]])
		off := s.StackTable.PrefixOffset[st]
		if off == 0 {
			break
		}
		st -= off
	}
	slices.Reverse(names)
	return names
}

func TestPprofThreadsAndWeights(t *testing.T) {
	out := mustConvert(t, testProfile(), Options{})

	if got := len(out.Threads); got != 2 {
		t.Fatalf("threads = %d, want 2", got)
	}

	cnt, cpu := out.Threads[0], out.Threads[1]
	if cnt.Samples.WeightType != ffprof.WeightSamples || cpu.Samples.WeightType != ffprof.WeightTracingMs {
		t.Errorf("weight types = %q, %q", cnt.Samples.WeightType, cpu.Samples.WeightType)
	}
	// Zero-valued sample is dropped.
	if cpu.Samples.Length != 3 {
		t.Errorf("cpu samples = %d, want 3", cpu.Samples.Length)
	}
	if !slices.Equal(cpu.Samples.Weight, []float64{20, 10, 30}) {
		t.Errorf("cpu weights = %v", cpu.Samples.Weight)
	}
	if !slices.Equal(cpu.Samples.Time, []float64{0, 10, 20}) {
		t.Errorf("cpu times = %v", cpu.Samples.Time)
	}
	if out.Meta.Interval != 10 {
		t.Errorf("interval = %v", out.Meta.Interval)
	}
	if !slices.Equal(out.Meta.InitialSelectedThreads, []int{1}) {
		t.Errorf("selected = %v, want last sample type", out.Meta.InitialSelectedThreads)
	}
	if !cnt.IsMainThread || !cpu.IsMainThread || cnt.Pid == cpu.Pid {
		t.Errorf("each sample type must be its own process")
	}
}

func TestPprofStacks(t *testing.T) {
	out := mustConvert(t, testProfile(), Options{})
	cpu := out.Threads[1]

	want := [][]string{
		{"main.main", "main.work", "main.helper", "runtime.mallocgc"},
		{"main.main", "main.work", "main.helper"},
		{"main.main", "github.com/x/y.(*T).rec[...]", "github.com/x/y.(*T).rec[...]"},
	}
	for i, w := range want {
		if got := stackNames(out, cpu.Samples.Stack[i]); !slices.Equal(got, w) {
			t.Errorf("sample %d stack = %v, want %v", i, got, w)
		}
	}

	// Stacks are shared between sample types.
	if !slices.Equal(out.Threads[0].Samples.Stack, cpu.Samples.Stack) {
		t.Errorf("stacks differ between sample types")
	}
}

func TestPprofFrames(t *testing.T) {
	out := mustConvert(t, testProfile(), Options{})
	s := &out.Shared

	byName := map[string]int{}
	for i, f := range s.FrameTable.Func {
		byName[s.StringArray[s.FuncTable.Name[f]]] = i
	}

	work, helper := byName["main.work"], byName["main.helper"]
	if s.FrameTable.Flags[work]&ffprof.FrameIsInlined != 0 {
		t.Errorf("outer frame marked inlined")
	}
	if s.FrameTable.Flags[helper]&ffprof.FrameIsInlined == 0 {
		t.Errorf("inner frame not marked inlined")
	}
	if s.FrameTable.Address[work] != 0x200 || s.FrameTable.Address[helper] != 0x200 {
		t.Errorf("addresses = %#x, %#x, want 0x200", s.FrameTable.Address[work], s.FrameTable.Address[helper])
	}
	if len(out.Libs) != 1 || out.Libs[0].BreakpadID != "abc" {
		t.Errorf("libs = %+v", out.Libs)
	}

	cats := map[string]int32{
		"main.main":                    catUser,
		"runtime.mallocgc":             catRuntime,
		"github.com/x/y.(*T).rec[...]": catUser,
	}
	for name, c := range cats {
		if got := s.FrameTable.Category[byName[name]]; got != c {
			t.Errorf("%s category = %d, want %d", name, got, c)
		}
	}

	rec := s.FrameTable.Func[byName["github.com/x/y.(*T).rec[...]"]]
	if res := s.StringArray[s.ResourceTable.Name[s.FuncTable.Resource[rec]]]; res != "github.com/x/y" {
		t.Errorf("resource = %q", res)
	}
}

func TestPprofGroupBy(t *testing.T) {
	out := mustConvert(t, testProfile(), Options{GroupBy: "job"})

	var names []string
	for _, th := range out.Threads[:3] {
		names = append(names, th.Name)
	}
	if want := []string{"job=a", "job=b", "job=" + noLabel}; !slices.Equal(names, want) {
		t.Errorf("threads = %v, want %v", names, want)
	}
	if len(out.Threads) != 6 {
		t.Errorf("threads = %d, want 6", len(out.Threads))
	}
	if !out.Threads[0].IsMainThread || out.Threads[1].IsMainThread {
		t.Errorf("first group thread must be main")
	}
}

func TestGoPackage(t *testing.T) {
	cases := map[string]string{
		"runtime.mallocgc":                    "runtime",
		"main.main.func1":                     "main",
		"github.com/a/b.(*T).M":               "github.com/a/b",
		"github.com/a/b.F[go.shape.*uint8]":   "github.com/a/b",
		"slices.Sort[[]github.com/x/y.T,...]": "slices",
		"__libc_start_main":                   "",
		"internal/runtime/syscall.Syscall6":   "internal/runtime/syscall",
		"gopkg.in/yaml%2ev3.(*parser).parse":  "gopkg.in/yaml%2ev3",
	}
	for in, want := range cases {
		if got := goPackage(in); got != want {
			t.Errorf("goPackage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]int32{
		"runtime.gcBgMarkWorker":            catGC,
		"runtime.scanobject":                catGC,
		"runtime.mallocgc":                  catRuntime,
		"internal/runtime/maps.(*Map).Get":  catRuntime,
		"syscall.Syscall":                   catSyscall,
		"internal/runtime/syscall.Syscall6": catSyscall,
		"main.main":                         catUser,
		"__libc_start_main":                 catOther,
	}
	for fn, want := range cases {
		if got := classify(fn, goPackage(fn)); got != want {
			t.Errorf("classify(%q) = %d, want %d", fn, got, want)
		}
	}
}

// TestRealProfiles converts CPU and heap profiles produced by runtime/pprof.
func TestRealProfiles(t *testing.T) {
	var cpu bytes.Buffer
	if err := pprof.StartCPUProfile(&cpu); err != nil {
		t.Fatal(err)
	}
	burn(200 * time.Millisecond)
	pprof.StopCPUProfile()

	runtime.GC()
	var heap bytes.Buffer
	if err := pprof.Lookup("heap").WriteTo(&heap, 0); err != nil {
		t.Fatal(err)
	}

	for name, buf := range map[string]*bytes.Buffer{"cpu": &cpu, "heap": &heap} {
		t.Run(name, func(t *testing.T) {
			p, err := profile.Parse(buf)
			if err != nil {
				t.Fatal(err)
			}
			out := mustConvert(t, p, Options{})
			if len(out.Threads) != len(p.SampleType) {
				t.Errorf("threads = %d, want %d", len(out.Threads), len(p.SampleType))
			}
		})
	}
}

var sink []byte

func burn(d time.Duration) {
	for end := time.Now().Add(d); time.Now().Before(end); {
		h := sha256.Sum256(sink)
		sink = append(make([]byte, 0, 64), h[:]...)
	}
}
