# Benchmarks: v1/runtime/plugin/codec/integer

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-03 | C37 | integer.to_bytes width 2 | n/a (not measured); p n/a (not measured) | 136→136 | 9→8 | `<bin>.test -test.run '^$' -test.bench 'ToBytes_Builtin' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | integer.to_bytes width 8 | n/a (not measured); p n/a (not measured) | 144→136 | 9→8 | `<bin>.test -test.run '^$' -test.bench 'ToBytes_Builtin' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | integer.to_bytes width 16 | n/a (not measured); p n/a (not measured) | 160→144 | 9→8 | `<bin>.test -test.run '^$' -test.bench 'ToBytes_Builtin' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-04 | C38c | FromBytes/2 | 277→44 (impl); tester -87% (p=0.002) | 88→0 | 6→0 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
| 2026-11-04 | C38c | FromBytes/8 | 348→178 (impl, p=0.002); tester -54% (p=0.026) | 128→40 | 8→2 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
| 2026-11-04 | C38c | FromBytes/16 | not significant (impl); tester p=0.065 | 209→128 | 8→3 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
| 2026-11-04 | C38c | ToBytes/2 | 676→158 (impl, p=0.002); tester -78% (p<=0.004) | 136→18 | 8→2 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
| 2026-11-04 | C38c | ToBytes/8 | 582→178 (impl, p=0.002); tester -76% (p<=0.004) | 136→24 | 8→2 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
| 2026-11-04 | C38c | ToBytes/16 | not significant (impl); tester -53% (p<=0.004) | 144→72 | 8→4 | `<bin> -test.run '^$' -test.bench Builtin -test.benchmem -test.count 1` (`go test -c ./v1/runtime/plugin/codec/integer`, base and W, 6 interleaved rounds, then `go tool benchstat base.txt w.txt`) | i7-12800H WSL2 (load 26-40) |
