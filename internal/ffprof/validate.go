package ffprof

import (
	"fmt"
	"reflect"
)

// Validate checks table column lengths and index ranges.
func Validate(p *Profile) error {
	s := &p.Shared
	tables := map[string]any{
		"stackTable":          &s.StackTable,
		"frameTable":          &s.FrameTable,
		"funcTable":           &s.FuncTable,
		"resourceTable":       &s.ResourceTable,
		"nativeSymbols":       &s.NativeSymbols,
		"sources":             &s.Sources,
		"sourceLocationTable": &s.SourceLocationTable,
	}
	for i, t := range p.Threads {
		tables[fmt.Sprintf("threads[%d].samples", i)] = &t.Samples
		tables[fmt.Sprintf("threads[%d].markers", i)] = &t.Markers
	}

	for name, t := range tables {
		if err := checkColumns(t); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	return checkIndices(p)
}

// checkColumns ensures every slice field has len == Length and is non-nil.
func checkColumns(table any) error {
	v := reflect.ValueOf(table).Elem()
	n := int(v.FieldByName("Length").Int())

	for i := range v.NumField() {
		f := v.Field(i)
		if f.Kind() != reflect.Slice {
			continue
		}

		name := v.Type().Field(i).Name
		if f.IsNil() {
			return fmt.Errorf("column %s is nil", name)
		}
		if f.Len() != n {
			return fmt.Errorf("column %s has %d rows, want %d", name, f.Len(), n)
		}
	}
	return nil
}

func checkIndices(p *Profile) error {
	s := &p.Shared
	inRange := func(what string, i int32, n int) error {
		if i < 0 || int(i) >= n {
			return fmt.Errorf("%s index %d out of range [0,%d)", what, i, n)
		}
		return nil
	}

	for i, off := range s.StackTable.PrefixOffset {
		if off < 0 || int(off) > i {
			return fmt.Errorf("stack %d: bad prefixOffset %d", i, off)
		}
		if err := inRange("stack frame", s.StackTable.Frame[i], s.FrameTable.Length); err != nil {
			return err
		}
	}

	ncat := len(p.Meta.Categories)
	for i, fn := range s.FrameTable.Func {
		if err := inRange("frame func", fn, s.FuncTable.Length); err != nil {
			return err
		}
		if err := inRange("frame category", s.FrameTable.Category[i], ncat); err != nil {
			return err
		}
		if s.FrameTable.Flags[i]&FrameHasAddress == 0 {
			continue
		}
		if err := inRange("frame lib", s.FrameTable.Lib[i], len(p.Libs)); err != nil {
			return err
		}
	}

	nstr := len(s.StringArray)
	for i, name := range s.FuncTable.Name {
		if err := inRange("func name", name, nstr); err != nil {
			return err
		}
		flags := s.FuncTable.Flags[i]
		if flags&FuncHasResource != 0 {
			if err := inRange("func resource", s.FuncTable.Resource[i], s.ResourceTable.Length); err != nil {
				return err
			}
		}
		if flags&FuncHasSource != 0 {
			if err := inRange("func source", s.FuncTable.Source[i], s.Sources.Length); err != nil {
				return err
			}
		}
	}

	for _, t := range p.Threads {
		for _, st := range t.Samples.Stack {
			if err := inRange("sample stack", st, s.StackTable.Length); err != nil {
				return err
			}
		}
	}
	return nil
}
