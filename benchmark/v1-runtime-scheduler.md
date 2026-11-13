# Benchmarks: v1/runtime/scheduler

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-02 | C33 | Beside_NoReporter/256 | n/a (not measured); p n/a (not measured) | n/a (not measured) | 5921→5155 (implementer; tester did not re-measure; per spawn 23→20) | `<bin>.test -test.bench Beside_NoReporter -test.benchmem -test.benchtime 200ms` (6 interleaved rounds) | i7-12800H WSL2 |
| 2026-11-05 | C53 | Spawn_Join/256 | 2.449ms→2.310ms (-5.7%, p=0.002, n=12, load 3 to 4) | 663.7Ki→669.9Ki (+0.9%) | 7845→8009 (+2.1%) | `<bin>.test -test.run '^$' -test.bench BenchmarkSpawn_Join -test.benchmem -test.benchtime 200x` (12 interleaved rounds, benchstat) | i7-12800H WSL2 6.18 |
