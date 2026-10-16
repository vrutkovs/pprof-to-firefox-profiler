package ffprof

// NoLib marks a frame without a native library / address.
const NoLib = -1

// NoStack is the prefix of a root stack.
const NoStack = -1

// Func describes a function before interning.
type Func struct {
	Name     string
	File     string
	Line     int32
	Resource string // e.g. Go package
}

// Frame describes a frame before interning. Func is a Builder.Func index.
type Frame struct {
	Func     int32
	Line     int32
	Category int32
	Inlined  bool
	Lib      int32 // NoLib if unknown
	Address  uint32
}

type libKey struct{ path, buildID string }

// Builder interns strings, funcs, frames and stacks into shared tables.
type Builder struct {
	p *Profile

	strings   map[string]int32
	sources   map[string]int32
	resources map[string]int32
	libs      map[libKey]int32
	funcs     map[Func]int32
	frames    map[Frame]int32
	stacks    map[[2]int32]int32
}

// NewBuilder returns a Builder for a profile with given meta.
// Version fields and required empty columns are filled in.
func NewBuilder(meta Meta) *Builder {
	meta.Version = GeckoVersion
	meta.PreprocessedProfileVersion = ProcessedVersion
	if meta.MarkerSchema == nil {
		meta.MarkerSchema = []any{}
	}

	return &Builder{
		p: &Profile{
			Meta:     meta,
			Libs:     []Lib{},
			Pages:    []any{},
			Counters: []any{},
			Threads:  []*Thread{},
			Shared:   emptyShared(),
		},
		strings:   map[string]int32{},
		sources:   map[string]int32{},
		resources: map[string]int32{},
		libs:      map[libKey]int32{},
		funcs:     map[Func]int32{},
		frames:    map[Frame]int32{},
		stacks:    map[[2]int32]int32{},
	}
}

func emptyShared() Shared {
	return Shared{
		StackTable: StackTable{Frame: []int32{}, PrefixOffset: []int32{}},
		FrameTable: FrameTable{
			Flags: []int32{}, Address: []uint32{}, Lib: []int32{},
			Category: []int32{}, Subcategory: []int32{}, Func: []int32{},
			NativeSymbol: []int32{}, InnerWindowID: []float64{},
			Line: []int32{}, Column: []int32{}, OriginalLocation: []int32{},
		},
		FuncTable: FuncTable{
			Flags: []int32{}, Name: []int32{}, Resource: []int32{}, Source: []int32{},
			LineNumber: []int32{}, ColumnNumber: []int32{}, OriginalLocation: []int32{},
		},
		ResourceTable: ResourceTable{Name: []int32{}, Host: []*int32{}, Type: []int{}},
		NativeSymbols: NativeSymbolTable{
			LibIndex: []int32{}, Address: []uint32{}, Name: []int32{}, FunctionSize: []int32{},
		},
		StringArray: []string{},
		Sources: SourceTable{
			ID: []*string{}, Filename: []int32{}, StartLine: []int32{},
			StartColumn: []int32{}, SourceMapURL: []*int32{}, Content: []*string{},
		},
		SourceLocationTable: SourceLocationTable{Source: []int32{}, Line: []int32{}, Column: []int32{}},
	}
}

// Profile returns the built profile.
func (b *Builder) Profile() *Profile { return b.p }

// AddThread appends a thread and returns its index.
func (b *Builder) AddThread(t *Thread) int {
	b.p.Threads = append(b.p.Threads, t)
	return len(b.p.Threads) - 1
}

// String interns s into the shared string array.
func (b *Builder) String(s string) int32 {
	if i, ok := b.strings[s]; ok {
		return i
	}

	sa := &b.p.Shared.StringArray
	i := int32(len(*sa))
	*sa = append(*sa, s)
	b.strings[s] = i
	return i
}

// Lib interns a library, deduplicated by path and build ID.
func (b *Builder) Lib(l Lib) int32 {
	k := libKey{l.Path, l.BreakpadID}
	if i, ok := b.libs[k]; ok {
		return i
	}

	i := int32(len(b.p.Libs))
	b.p.Libs = append(b.p.Libs, l)
	b.libs[k] = i
	return i
}

func (b *Builder) source(file string) int32 {
	if i, ok := b.sources[file]; ok {
		return i
	}

	t := &b.p.Shared.Sources
	i := int32(t.Length)
	t.ID = append(t.ID, nil)
	t.Filename = append(t.Filename, b.String(file))
	t.StartLine = append(t.StartLine, 1)
	t.StartColumn = append(t.StartColumn, 1)
	t.SourceMapURL = append(t.SourceMapURL, nil)
	t.Content = append(t.Content, nil)
	t.Length++
	b.sources[file] = i
	return i
}

func (b *Builder) resource(name string) int32 {
	if i, ok := b.resources[name]; ok {
		return i
	}

	t := &b.p.Shared.ResourceTable
	i := int32(t.Length)
	t.Name = append(t.Name, b.String(name))
	t.Host = append(t.Host, nil)
	t.Type = append(t.Type, resourceTypeUnknown)
	t.Length++
	b.resources[name] = i
	return i
}

// Func interns a function. Empty File / Resource and zero Line are left unset.
func (b *Builder) Func(f Func) int32 {
	if i, ok := b.funcs[f]; ok {
		return i
	}

	var flags, res, src int32
	if f.Resource != "" {
		flags |= FuncHasResource
		res = b.resource(f.Resource)
	}
	if f.File != "" {
		flags |= FuncHasSource
		src = b.source(f.File)
	}
	if f.Line > 0 {
		flags |= FuncHasLine
	}

	t := &b.p.Shared.FuncTable
	i := int32(t.Length)
	t.Flags = append(t.Flags, flags)
	t.Name = append(t.Name, b.String(f.Name))
	t.Resource = append(t.Resource, res)
	t.Source = append(t.Source, src)
	t.LineNumber = append(t.LineNumber, f.Line)
	t.ColumnNumber = append(t.ColumnNumber, 0)
	t.OriginalLocation = append(t.OriginalLocation, 0)
	t.Length++
	b.funcs[f] = i
	return i
}

// Frame interns a frame. Category is always set.
func (b *Builder) Frame(f Frame) int32 {
	if i, ok := b.frames[f]; ok {
		return i
	}

	flags := int32(FrameHasCategory)
	lib, addr := int32(0), uint32(0)
	if f.Inlined {
		flags |= FrameIsInlined
	}
	if f.Lib != NoLib {
		flags |= FrameHasAddress
		lib, addr = f.Lib, f.Address
	}
	if f.Line > 0 {
		flags |= FrameHasLine
	}

	t := &b.p.Shared.FrameTable
	i := int32(t.Length)
	t.Flags = append(t.Flags, flags)
	t.Address = append(t.Address, addr)
	t.Lib = append(t.Lib, lib)
	t.Category = append(t.Category, f.Category)
	t.Subcategory = append(t.Subcategory, 0)
	t.Func = append(t.Func, f.Func)
	t.NativeSymbol = append(t.NativeSymbol, 0)
	t.InnerWindowID = append(t.InnerWindowID, 0)
	t.Line = append(t.Line, f.Line)
	t.Column = append(t.Column, 0)
	t.OriginalLocation = append(t.OriginalLocation, 0)
	t.Length++
	b.frames[f] = i
	return i
}

// Stack interns stack node (prefix, frame); prefix is NoStack for roots.
// Parents are always inserted before children, so prefixOffset stays positive.
func (b *Builder) Stack(prefix, frame int32) int32 {
	k := [2]int32{prefix, frame}
	if i, ok := b.stacks[k]; ok {
		return i
	}

	t := &b.p.Shared.StackTable
	i := int32(t.Length)
	off := int32(0)
	if prefix != NoStack {
		off = i - prefix
	}
	t.Frame = append(t.Frame, frame)
	t.PrefixOffset = append(t.PrefixOffset, off)
	t.Length++
	b.stacks[k] = i
	return i
}
