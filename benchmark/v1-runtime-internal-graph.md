# Benchmarks: v1/runtime/internal/graph

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-02 | C35b | BenchmarkEmit | n/a (not measured) | 1,584,914-1,587,073→1,459,775-1,462,491 (tester round 1) | 23,289→18,776-18,777 (tester round 2) | `go test -run '^$' -bench 'BenchmarkEmit$\|BenchmarkSamples$' -benchmem -count 1 .` (in v1/runtime/graph, base then W, 6 interleaved rounds) | n/a (not recorded) |
| 2026-11-02 | C35b | BenchmarkSamples | n/a (not measured) | 1,816,904-1,818,525→1,691,969-1,696,006 (tester round 1) | 27,146-27,168→22,634-22,648 (tester round 2) | `go test -run '^$' -bench 'BenchmarkEmit$\|BenchmarkSamples$' -benchmem -count 1 .` (in v1/runtime/graph, base then W, 6 interleaved rounds) | n/a (not recorded) |
