# Benchmarks: v1/runtime/plugin/base32

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-03 | C37 | base32.encode/16 | n/a (not measured); p n/a (not measured) | 48→48 | 2→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base32.encode/1024 | n/a (not measured); p n/a (not measured) | 3600→1808 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base32.encode/65536 | n/a (not measured); p n/a (not measured) | 213008→106512 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base32.decode/16 | n/a (not measured); p n/a (not measured) | 64→48 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base32.decode/1024 | n/a (not measured); p n/a (not measured) | 2832→1808 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base32.decode/65536 | n/a (not measured); p n/a (not measured) | 172048→106512 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
