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
- Extra tracks: `GC` (mark phase, stop-the-world), `Tasks` (all `runtime/trace` tasks), `No goroutine` (samples outside goroutines).
  Each task is also copied onto the goroutines that started and ended it. Its tooltip lists every goroutine that ran a region or log for it.
- Call tree: each goroutine state interval becomes a sample weighted by its duration, under a root frame naming the state:
  - `Waiting: <reason>`, `Syscall`: stack where the goroutine blocked
  - `Runnable (scheduler latency)`: stack where it last stopped
  - `Running`: real CPU samples (10 ms each) if a CPU profile ran during the trace; otherwise the stack where the
    goroutine stopped running, which is approximate: a hot loop is charged to wherever it later blocked.

  Off-CPU frames take the state's category color. `-stacks blocked` keeps off-CPU states only, `-stacks off` keeps CPU samples only.
  Use the call tree's "focus on function" on a root to look at one state.
- To get CPU samples, fetch a CPU profile while tracing:
  ```sh
  curl -s -o cpu.pprof "$URL/debug/pprof/profile?seconds=7" & sleep 1
  curl -s -o trace.out "$URL/debug/pprof/trace?seconds=5"; wait
  ```
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

## License

[Apache License 2.0](LICENSE)
