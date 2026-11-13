# Benchmarks: v1/runtime/plugin/random

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-03 | C37 | random.bytes x100 in a run/16 | n/a (not measured); p n/a (not measured) | 9288→9288 | 452→452 (no change) | `<bin>.test -test.run '^$' -test.bench 'Bytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | random.bytes x100 in a run/1024 | n/a (not measured); p n/a (not measured) | 212489→110089 | 552→452 | `<bin>.test -test.run '^$' -test.bench 'Bytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | random.bytes x100 in a run/65536 | n/a (not measured); p n/a (not measured) | 13114955→6561335 | 552→452 | `<bin>.test -test.run '^$' -test.bench 'Bytes' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
