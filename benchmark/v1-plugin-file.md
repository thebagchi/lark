# Benchmarks: v1/plugin/file

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-03 | C37 | file.read, file.bytes (64 KiB file) | n/a (not measured); p n/a (not measured) | 139913→74377 | 10→9 | `<bin>.test -test.run '^$' -test.bench 'File_Read' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
