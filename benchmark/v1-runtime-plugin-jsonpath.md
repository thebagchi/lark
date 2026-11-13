# Benchmarks: v1/runtime/plugin/jsonpath

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-02 | C36 | Extract_Hit | n/a (not measured); p n/a (not measured) | 112→48 | 5→3 | `<binary> -test.run '^$' -test.bench Extract -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/jsonpath`, base and W, 6 interleaved rounds; tester ran `-test.bench Extract_`) | n/a (not recorded) |
| 2026-11-02 | C36 | Extract_HitDeep | n/a (not measured); p n/a (not measured) | 176→80 | 7→5 | `<binary> -test.run '^$' -test.bench Extract -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/jsonpath`, base and W, 6 interleaved rounds; tester ran `-test.bench Extract_`) | n/a (not recorded) |
| 2026-11-02 | C36 | Extract_HitEscaped | n/a (not measured); p n/a (not measured) | 144→80 | 8→6 | `<binary> -test.run '^$' -test.bench Extract -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/jsonpath`, base and W, 6 interleaved rounds; tester ran `-test.bench Extract_`) | n/a (not recorded) |
| 2026-11-02 | C36 | Extract_Miss | n/a (not measured); p n/a (not measured) | 200→48 | 9→3 | `<binary> -test.run '^$' -test.bench Extract -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/jsonpath`, base and W, 6 interleaved rounds; tester ran `-test.bench Extract_`) | n/a (not recorded) |
| 2026-11-02 | C36 | Extract_MissDeep | n/a (not measured); p n/a (not measured) | 288→80 | 12→5 | `<binary> -test.run '^$' -test.bench Extract -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/jsonpath`, base and W, 6 interleaved rounds; tester ran `-test.bench Extract_`) | n/a (not recorded) |
| 2026-11-02 | C36b | BenchmarkPatch_AddMany | n/a (not measured); p n/a (not measured) | 56408→46904 | 1922→1130 | `go test -run '^$' -bench 'BenchmarkPatch_(AddMany\|AddInList\|RemoveInList)$' -benchmem -count 6 ./v1/runtime/plugin/jsonpath` (before and after run in sequence, not interleaved) | i7-12800H WSL2 |
| 2026-11-02 | C36b | BenchmarkPatch_AddInList (control, no change) | n/a (not measured) | 17168→17168 | 15→15 | `go test -run '^$' -bench 'BenchmarkPatch_(AddMany\|AddInList\|RemoveInList)$' -benchmem -count 6 ./v1/runtime/plugin/jsonpath` (before and after run in sequence, not interleaved) | i7-12800H WSL2 |
| 2026-11-02 | C36b | BenchmarkPatch_RemoveInList (control, no change) | n/a (not measured) | 17184→17184 | 16→16 | `go test -run '^$' -bench 'BenchmarkPatch_(AddMany\|AddInList\|RemoveInList)$' -benchmem -count 6 ./v1/runtime/plugin/jsonpath` (before and after run in sequence, not interleaved) | i7-12800H WSL2 |
