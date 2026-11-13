# Benchmarks: v1/runtime/plugin/base64

This table holds one row per benchmark for each accepted change that claimed a speedup in this package. Numbers come from the reports of the change. "n/a (not measured)" means the report gives no figure.
Rows only append, newest last.

| Date | Change | Benchmark | ns/op before→after (p) | B/op | allocs/op | Command | Machine |
|---|---|---|---|---|---|---|---|
| 2026-11-03 | C37 | base64.encode/16 | n/a (not measured); p n/a (not measured) | 40→40 | 2→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base64.encode/1024 | n/a (not measured); p n/a (not measured) | 2832→1424 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base64.encode/65536 | n/a (not measured); p n/a (not measured) | 180241→90129 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base64.decode/16 | n/a (not measured); p n/a (not measured) | 48→32 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base64.decode/1024 | n/a (not measured); p n/a (not measured) | 2064→1040 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-03 | C37 | base64.decode/65536 | n/a (not measured); p n/a (not measured) | 131088→65552 | 3→2 | `<bin>.test -test.run '^$' -test.bench 'Encode\|Decode' -test.benchmem -test.count 1` (base and W test binaries, 6 interleaved rounds) | WSL2 host, linux/amd64, 8 CPU |
| 2026-11-04 | C51 base64 kernels | Encode/1024 | 1118→435 (p=0.002) | 1424→1424 | 2→2 | `call.base.test` vs `call.new.test`, `-test.bench '^Benchmark(Encode\|Decode)$' -test.benchmem -test.benchtime 300ms`, 6 interleaved rounds, benchstat | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Encode/65536 | 61090→21260 (p=0.002) | 90132→90132 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Encode/1048576 | 1081000→411900 (p=0.002) | 1400832→1400832 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Decode/1024 | 1750→1172 (p=0.002) | 1040→1040 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Decode/65536 | 107390→73330 (p=0.002) | 65556→65556 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Decode/1048576 | 1736000→1058000 (p=0.002) | 1048576→1048576 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Encode/16 (no kernel runs; no change) | 66.2→65.0 (p=0.818) | 40→40 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 base64 kernels | Decode/16 (no kernel runs; slower, noise and layout) | 91.2→116.6 (p=0.002) | 32→32 | 2→2 | same | i7-12800H WSL2 |
| 2026-11-04 | C51 AVX2 encode over SSE | Encode/512 | −26% (p=0.029) | n/a | n/a | tester: W with AVX2 off vs shipped W, 15 rounds, benchstat (C51.test2.md) | i7-12800H WSL2 |
| 2026-11-04 | C51 AVX2 encode over SSE | Encode/4096 | −30% (p=0.007) | n/a | n/a | same | i7-12800H WSL2 |
| 2026-11-04 | C51 AVX2 decode over SSE (no gain) | Decode/256 | 853→853 (p=0.91) | n/a | n/a | tester: W with AVX2 off vs shipped W, 30 rounds, benchstat (C51.test2.md) | i7-12800H WSL2 |
| 2026-11-04 | C51 AVX2 decode over SSE (no gain) | Decode/1024 | 2653→2735 (p=0.57) | n/a | n/a | same | i7-12800H WSL2 |
| 2026-11-04 | C51 AVX2 decode over SSE (no gain) | Decode/4096 | 10069→9682 (p=0.096) | n/a | n/a | same | i7-12800H WSL2 |
