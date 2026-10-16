# pprof2ff

Convert Go [pprof](https://github.com/google/pprof) profiles and execution traces into the
[Firefox Profiler](https://profiler.firefox.com) processed format (v75).
See [profiler#4462](https://github.com/firefox-devtools/profiler/issues/4462).

```sh
go install github.com/vrutkovs/firefox-profiler-pprof@latest   # installs as firefox-profiler-pprof
go build -o pprof2ff .                                           # or build as pprof2ff

pprof2ff -open cpu.pprof                      # open in profiler.firefox.com
pprof2ff -o cpu.json.gz cpu.pprof             # write gzipped JSON
pprof2ff -group-by worker -open cpu.pprof     # one track per pprof label value
curl -s localhost:6060/debug/pprof/heap | pprof2ff -o heap.json -

go test -trace trace.out -cpuprofile cpu.out ./pkg   # CPU samples land in the trace
pprof2ff -open trace.out
pprof2ff -min-state 100us -open trace.out            # drop short state markers on busy traces
```

The input format is detected by its header.

## pprof mapping

- Each pprof sample type (e.g. `alloc_space`, `inuse_space`) becomes its own process track.
- `-group-by LABEL` splits samples into threads by label value. Samples without the label go to `<none>`.
- Weights: time units become `tracing-ms`, `bytes` stays bytes, everything else counts as samples.
- Inlined functions become inlined frames. Go packages become resources, so you can collapse a whole package.
- Frame categories: User, Go runtime, GC, Syscall, Other.

## Execution trace mapping

Traces from Go 1.11 and later are read with `golang.org/x/exp/trace`.

- One process with one thread per goroutine, named after its outermost non-stdlib function, e.g. `main.worker #42`.
  Goroutines that ran less than `-min-goroutine` (default 1ms) and have no CPU samples are hidden; enable them from the track menu.
- Extra tracks: `GC` (mark phase, stop-the-world), `Tasks` (`runtime/trace` tasks), `No goroutine` (samples outside goroutines).
- CPU samples become samples at their real time. They only exist if a CPU profile was running during the trace.
- Markers:
  - Goroutine states `Running`, `Runnable`, `Waiting`, `Syscall`, with reason and P/M. Waiting and syscall markers carry the blocking stack.
  - `runtime/trace` regions and logs on their goroutine.
  - Mark assists and incremental sweeps on the goroutine that did them.
- Counters: heap live, heap goal, GOMAXPROCS, goroutine count.

## Limits

- pprof has no timestamps. The timeline is synthetic: samples in order, one `interval` apart. Use the call tree, flame graph and inverted views.
- Busy traces produce millions of state markers, e.g. 400 goroutines doing channel ping-pong for 0.3 s give 4.8M markers. Use `-min-state 10us` or more.
- Traces don't record the CPU profile rate; the default 100 Hz is assumed.
- `-open` serves the profile once on `127.0.0.1` and exits after the profiler fetches it, or after 2 minutes.
