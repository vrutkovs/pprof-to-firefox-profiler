package convert

import (
	"strings"

	"github.com/vrutkovs/firefox-profiler-pprof/internal/ffprof"
)

// Category indices into categories. Other must stay first and grey: the profiler
// uses the first grey category as default.
const (
	catOther = iota
	catUser
	catRuntime
	catGC
	catSyscall
	catBlocked
)

var categories = []ffprof.Category{
	catOther:   {Name: "Other", Color: "grey", Subcategories: []string{"Other"}},
	catUser:    {Name: "User", Color: "blue", Subcategories: []string{"Other"}},
	catRuntime: {Name: "Go runtime", Color: "orange", Subcategories: []string{"Other"}},
	catGC:      {Name: "GC", Color: "green", Subcategories: []string{"Other"}},
	catSyscall: {Name: "Syscall", Color: "red", Subcategories: []string{"Other"}},
	catBlocked: {Name: "Blocked", Color: "yellow", Subcategories: []string{"Other"}},
}

// gcPrefixes match runtime functions doing GC work: marking, sweeping, scavenging, assists.
var gcPrefixes = []string{
	"runtime.gc",
	"runtime.bgsweep",
	"runtime.bgscavenge",
	"runtime.markroot",
	"runtime.scanobject",
	"runtime.scanblock",
	"runtime.scanstack",
	"runtime.scanframeworker",
	"runtime.greyobject",
	"runtime.sweepone",
	"runtime.wbBuf",
	"runtime.(*gcWork)",
	"runtime.(*gcControllerState)",
	"runtime.(*mspan).sweep",
	"runtime.(*sweepLocked)",
	"runtime.(*scavengerState)",
	"runtime.(*pageAlloc).scavenge",
}

func classify(fn, pkg string) int32 {
	for _, p := range gcPrefixes {
		if strings.HasPrefix(fn, p) {
			return catGC
		}
	}

	switch {
	case pkg == "":
		return catOther
	case pkg == "syscall",
		strings.HasPrefix(pkg, "internal/syscall/"),
		pkg == "internal/runtime/syscall",
		pkg == "golang.org/x/sys/unix",
		pkg == "golang.org/x/sys/windows":
		return catSyscall
	case pkg == "runtime",
		strings.HasPrefix(pkg, "runtime/internal/"),
		strings.HasPrefix(pkg, "internal/runtime/"):
		return catRuntime
	}
	return catUser
}

// goPackage extracts the import path from a Go symbol, "" if it isn't one.
//
//	github.com/a/b.(*T).M[...]  -> github.com/a/b
//	runtime.mallocgc            -> runtime
//	__libc_start_main           -> ""
func goPackage(fn string) string {
	// Generic type args may contain dots and slashes.
	if i := strings.IndexByte(fn, '['); i >= 0 {
		fn = fn[:i]
	}

	slash := strings.LastIndexByte(fn, '/') + 1
	dot := strings.IndexByte(fn[slash:], '.')
	if dot <= 0 {
		return ""
	}
	return fn[:slash+dot]
}
