# pprof2ff

Convert Go [pprof](https://github.com/google/pprof) profiles into the
[Firefox Profiler](https://profiler.firefox.com) processed format (v75).
See [profiler#4462](https://github.com/firefox-devtools/profiler/issues/4462).

```sh
go install github.com/vrutkovs/firefox-profiler-pprof@latest   # installs as firefox-profiler-pprof
go build -o pprof2ff .                                           # or build as pprof2ff

pprof2ff -open cpu.pprof                      # open in profiler.firefox.com
pprof2ff -o cpu.json.gz cpu.pprof             # write gzipped JSON
pprof2ff -group-by worker -open cpu.pprof     # one track per pprof label value
curl -s localhost:6060/debug/pprof/heap | pprof2ff -o heap.json -
```

## Mapping

- Each pprof sample type (e.g. `alloc_space`, `inuse_space`) becomes its own process track.
- `-group-by LABEL` splits samples into threads by label value. Samples without the label go to `<none>`.
- Weights: time units become `tracing-ms`, `bytes` stays bytes, everything else counts as samples.
- Inlined functions become inlined frames. Go packages become resources, so you can collapse a whole package.
- Frame categories: User, Go runtime, GC, Syscall, Other.

## Limits

- pprof has no timestamps. The timeline is synthetic: samples in order, one `interval` apart. Use the call tree, flame graph and inverted views.
- `-open` serves the profile once on `127.0.0.1` and exits after the profiler fetches it, or after 2 minutes.
- Go execution traces are not supported yet.
