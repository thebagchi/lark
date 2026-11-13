# Benchmarks: v1/runtime/plugin/codec/binary

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-02 | C31 | FromInt/16 | n/a (not measured); p n/a (not measured) | 96→96 | 5→5 (no change) | `<bin>.test -test.run '^$' -test.bench 'FromBinary\|FromInt' -test.benchmem -test.count 1` (base and W test binaries from `go test -c`, 6 interleaved rounds) | n/a (not recorded) |
| 2026-11-02 | C31 | FromInt/1024 | n/a (not measured); p n/a (not measured) | 2144→1104 | 7→5 | `<bin>.test -test.run '^$' -test.bench 'FromBinary\|FromInt' -test.benchmem -test.count 1` (base and W test binaries from `go test -c`, 6 interleaved rounds) | n/a (not recorded) |
| 2026-11-02 | C31 | FromInt/65536 | n/a (not measured); p n/a (not measured) | 131169→65616 | 7→5 | `<bin>.test -test.run '^$' -test.bench 'FromBinary\|FromInt' -test.benchmem -test.count 1` (base and W test binaries from `go test -c`, 6 interleaved rounds) | n/a (not recorded) |
| 2026-11-03 | C37 | binary.to_bytes/16 | n/a (not measured); p n/a (not measured) | 48→32 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'ToBytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | binary.to_bytes/1024 | n/a (not measured); p n/a (not measured) | 2064→1040 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'ToBytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | binary.to_bytes/65536 | n/a (not measured); p n/a (not measured) | 131089→65552 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'ToBytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C38b | FromBytes/16 (binary.from_bytes) | n/a (not measured); tester benchstat ~ (p=0.132) | 160→144 | 3→2 | `./X.test -test.run '^$' -test.bench FromBytes -test.benchmem -test.benchtime 2000x` (`go test -c ./v1/runtime/plugin/codec/binary`, base and W, 6 interleaved rounds; tester ran benchstat) | WSL2, 8 CPUs (load 22-37) |
| 2026-11-03 | C38b | FromBytes/1024 (binary.from_bytes) | n/a (not measured); tester benchstat ~ (p=0.132) | 9232→8208 | 3→2 | `./X.test -test.run '^$' -test.bench FromBytes -test.benchmem -test.benchtime 2000x` (`go test -c ./v1/runtime/plugin/codec/binary`, base and W, 6 interleaved rounds; tester ran benchstat) | WSL2, 8 CPUs (load 22-37) |
| 2026-11-03 | C38b | FromBytes/65536 (binary.from_bytes) | n/a (not measured); tester benchstat ~ (p=0.394) | ~589850→~524310 | 3→2 | `./X.test -test.run '^$' -test.bench FromBytes -test.benchmem -test.benchtime 2000x` (`go test -c ./v1/runtime/plugin/codec/binary`, base and W, 6 interleaved rounds; tester ran benchstat) | WSL2, 8 CPUs (load 22-37) |
