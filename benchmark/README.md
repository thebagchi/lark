# Benchmarks

Dated reports from `./build.sh bench`, oldest first. Each section is one run.
The raw output of each run sits next to this file, named by its date.

Do not compare two sections to claim a speed-up. Runs taken hours apart
differ for reasons that are not the code. To compare, run both sides
together and give both raw files to benchstat:

    go tool benchstat benchmark/<before>.txt benchmark/<after>.txt

## Package reports

Each accepted change that claims a speedup adds a row to the report of its package.
The file name is the package path with slashes as dashes.
`./build.sh bench` regenerates this list from the files present.

- [v1/plugin/file](v1-plugin-file.md)
- [v1/runtime/internal/graph](v1-runtime-internal-graph.md)
- [v1/runtime/plugin/base32](v1-runtime-plugin-base32.md)
- [v1/runtime/plugin/base64](v1-runtime-plugin-base64.md)
- [v1/runtime/plugin/codec/binary](v1-runtime-plugin-codec-binary.md)
- [v1/runtime/plugin/codec/buf](v1-runtime-plugin-codec-buf.md)
- [v1/runtime/plugin/codec/hex](v1-runtime-plugin-codec-hex.md)
- [v1/runtime/plugin/codec/integer](v1-runtime-plugin-codec-integer.md)
- [v1/runtime/plugin/hash](v1-runtime-plugin-hash.md)
- [v1/runtime/plugin/jsonpath](v1-runtime-plugin-jsonpath.md)
- [v1/runtime/plugin/random](v1-runtime-plugin-random.md)
- [v1/runtime/scheduler](v1-runtime-scheduler.md)
- [v1/runtime](v1-runtime.md)

## 2026-11-01-134827

- Command: `go test -run '^$' -bench . -benchmem -count 6 ./v1/...`
- Commit: cc05da8, with uncommitted changes
- Go: go1.26.10 linux/amd64
- CPU: 12th Gen Intel(R) Core(TM) i7-12800H, 8 logical CPUs
- Kernel: 6.18.54.1-microsoft-standard-WSL2
- Raw: `2026-11-01-134827.txt`

```
goos: linux
goarch: amd64
pkg: github.com/thebagchi/lark/v1/runtime
cpu: 12th Gen Intel(R) Core(TM) i7-12800H
                         │ 2026-11-01-134827.txt │
                         │        sec/op         │
Run_Return-8                         14.44µ ± 1%
Evaluate_Return-8                    2.399µ ± 3%
Spawn_Join/1-8                       22.44µ ± 3%
Spawn_Join/16-8                      86.18µ ± 2%
Spawn_Join/256-8                     767.4µ ± 2%
Evaluate_SpawnJoin/1-8               16.35µ ± 1%
Evaluate_SpawnJoin/16-8              76.85µ ± 1%
Evaluate_SpawnJoin/256-8             767.8µ ± 5%
Call_Statements/1000-8               414.5µ ± 2%
Call_Statements/4000-8               1.549m ± 2%
Compile_Functions-8                  1.932m ± 1%
geomean                              118.0µ

                         │ 2026-11-01-134827.txt │
                         │         B/op          │
Run_Return-8                        3.078Ki ± 0%
Evaluate_Return-8                   3.188Ki ± 0%
Spawn_Join/1-8                      5.961Ki ± 0%
Spawn_Join/16-8                     38.07Ki ± 0%
Spawn_Join/256-8                    518.1Ki ± 0%
Evaluate_SpawnJoin/1-8              6.266Ki ± 0%
Evaluate_SpawnJoin/16-8             38.38Ki ± 0%
Evaluate_SpawnJoin/256-8            518.4Ki ± 0%
Call_Statements/1000-8              160.0Ki ± 0%
Call_Statements/4000-8              628.7Ki ± 0%
Compile_Functions-8                 1.525Mi ± 0%
geomean                             57.47Ki

                         │ 2026-11-01-134827.txt │
                         │       allocs/op       │
Run_Return-8                          40.00 ± 0%
Evaluate_Return-8                     44.00 ± 0%
Spawn_Join/1-8                        93.00 ± 0%
Spawn_Join/16-8                       523.0 ± 0%
Spawn_Join/256-8                     7.023k ± 0%
Evaluate_SpawnJoin/1-8                101.0 ± 0%
Evaluate_SpawnJoin/16-8               531.0 ± 0%
Evaluate_SpawnJoin/256-8             7.031k ± 0%
Call_Statements/1000-8               3.056k ± 0%
Call_Statements/4000-8               12.06k ± 0%
Compile_Functions-8                  23.37k ± 0%
geomean                               862.0

pkg: github.com/thebagchi/lark/v1/runtime/plugin/base64
               │ 2026-11-01-134827.txt │
               │        sec/op         │
Encode/16-8                48.14n ± 1%
Encode/1024-8              1.012µ ± 1%
Encode/65536-8             56.41µ ± 2%
geomean                    1.400µ

               │ 2026-11-01-134827.txt │
               │         B/op          │
Encode/16-8                 40.00 ± 0%
Encode/1024-8             2.766Ki ± 0%
Encode/65536-8            176.0Ki ± 0%
geomean                   2.669Ki

               │ 2026-11-01-134827.txt │
               │       allocs/op       │
Encode/16-8                 2.000 ± 0%
Encode/1024-8               3.000 ± 0%
Encode/65536-8              3.000 ± 0%
geomean                     2.621

               │ 2026-11-01-134827.txt │
               │          B/s          │
Encode/16-8               317.0Mi ± 1%
Encode/1024-8             965.7Mi ± 1%
Encode/65536-8            1.082Gi ± 2%
geomean                   697.4Mi

pkg: github.com/thebagchi/lark/v1/runtime/plugin/codec/binary
               │ 2026-11-01-134827.txt │
               │        sec/op         │
Encode/16-8                45.90n ± 1%
Encode/1024-8              2.237µ ± 1%
Encode/65536-8             113.9µ ± 3%
Decode/16-8                34.31n ± 2%
Decode/1024-8              1.106µ ± 4%
Decode/65536-8             67.62µ ± 1%
geomean                    1.763µ

               │ 2026-11-01-134827.txt │
               │         B/op          │
Encode/16-8                 128.0 ± 0%
Encode/1024-8             8.000Ki ± 0%
Encode/65536-8            512.0Ki ± 0%
Decode/16-8                 16.00 ± 0%
Decode/1024-8             1.000Ki ± 0%
Decode/65536-8            64.00Ki ± 0%
geomean                   2.828Ki

               │ 2026-11-01-134827.txt │
               │       allocs/op       │
Encode/16-8                 1.000 ± 0%
Encode/1024-8               1.000 ± 0%
Encode/65536-8              1.000 ± 0%
Decode/16-8                 1.000 ± 0%
Decode/1024-8               1.000 ± 0%
Decode/65536-8              1.000 ± 0%
geomean                     1.000

               │ 2026-11-01-134827.txt │
               │          B/s          │
Encode/16-8               332.5Mi ± 1%
Encode/1024-8             436.5Mi ± 1%
Encode/65536-8            549.1Mi ± 3%
Decode/16-8               3.474Gi ± 2%
Decode/1024-8             6.899Gi ± 4%
Decode/65536-8            7.221Gi ± 1%
geomean                   1.530Gi

pkg: github.com/thebagchi/lark/v1/runtime/plugin/codec/hex
                  │ 2026-11-01-134827.txt │
                  │        sec/op         │
FromBytes/16-8               51.38n ±  1%
FromBytes/1024-8             1.353µ ± 11%
FromBytes/65536-8            78.56µ ±  3%
ToBytes/16-8                 71.36n ±  3%
ToBytes/1024-8               1.121µ ±  2%
ToBytes/65536-8              60.49µ ±  2%
geomean                      1.726µ

                  │ 2026-11-01-134827.txt │
                  │         B/op          │
FromBytes/16-8                 48.00 ± 0%
FromBytes/1024-8             2.016Ki ± 0%
FromBytes/65536-8            128.0Ki ± 0%
ToBytes/16-8                   48.00 ± 0%
ToBytes/1024-8               2.016Ki ± 0%
ToBytes/65536-8              128.0Ki ± 0%
geomean                      2.295Ki

                  │ 2026-11-01-134827.txt │
                  │       allocs/op       │
FromBytes/16-8                 2.000 ± 0%
FromBytes/1024-8               2.000 ± 0%
FromBytes/65536-8              2.000 ± 0%
ToBytes/16-8                   3.000 ± 0%
ToBytes/1024-8                 3.000 ± 0%
ToBytes/65536-8                3.000 ± 0%
geomean                        2.449

                  │ 2026-11-01-134827.txt │
                  │          B/s          │
FromBytes/16-8              297.0Mi ±  1%
FromBytes/1024-8            722.1Mi ± 10%
FromBytes/65536-8           795.5Mi ±  3%
ToBytes/16-8                427.7Mi ±  3%
ToBytes/1024-8              1.702Gi ±  2%
ToBytes/65536-8             2.018Gi ±  2%
geomean                     800.3Mi

pkg: github.com/thebagchi/lark/v1/runtime/plugin/codec/integer
                            │ 2026-11-01-134827.txt │
                            │        sec/op         │
Encode/unsigned/big/1-8                 22.79n ± 2%
Encode/unsigned/big/4-8                 23.98n ± 2%
Encode/unsigned/big/8-8                 25.82n ± 2%
Encode/unsigned/big/16-8                36.59n ± 2%
Encode/unsigned/little/1-8              22.56n ± 1%
Encode/unsigned/little/4-8              23.06n ± 2%
Encode/unsigned/little/8-8              24.79n ± 4%
Encode/unsigned/little/16-8             61.02n ± 3%
Encode/signed/big/8-8                   25.52n ± 2%
Encode/signed/little/8-8                25.75n ± 2%
Decode/unsigned/big/1-8                 32.62n ± 1%
Decode/unsigned/big/4-8                 32.79n ± 7%
Decode/unsigned/big/8-8                 32.08n ± 4%
Decode/unsigned/big/16-8                44.52n ± 2%
Decode/unsigned/little/1-8              32.43n ± 1%
Decode/unsigned/little/4-8              32.41n ± 1%
Decode/unsigned/little/8-8              32.27n ± 2%
Decode/unsigned/little/16-8             69.62n ± 1%
Decode/signed/big/8-8                   31.22n ± 2%
Decode/signed/little/8-8                31.60n ± 1%
geomean                                 31.54n

                            │ 2026-11-01-134827.txt │
                            │         B/op          │
Encode/unsigned/big/1-8                  1.000 ± 0%
Encode/unsigned/big/4-8                  4.000 ± 0%
Encode/unsigned/big/8-8                  8.000 ± 0%
Encode/unsigned/big/16-8                 16.00 ± 0%
Encode/unsigned/little/1-8               1.000 ± 0%
Encode/unsigned/little/4-8               4.000 ± 0%
Encode/unsigned/little/8-8               8.000 ± 0%
Encode/unsigned/little/16-8              32.00 ± 0%
Encode/signed/big/8-8                    8.000 ± 0%
Encode/signed/little/8-8                 8.000 ± 0%
Decode/unsigned/big/1-8                  40.00 ± 0%
Decode/unsigned/big/4-8                  40.00 ± 0%
Decode/unsigned/big/8-8                  40.00 ± 0%
Decode/unsigned/big/16-8                 80.00 ± 0%
Decode/unsigned/little/1-8               40.00 ± 0%
Decode/unsigned/little/4-8               40.00 ± 0%
Decode/unsigned/little/8-8               40.00 ± 0%
Decode/unsigned/little/16-8              96.00 ± 0%
Decode/signed/big/8-8                    40.00 ± 0%
Decode/signed/little/8-8                 40.00 ± 0%
geomean                                  16.27

                            │ 2026-11-01-134827.txt │
                            │       allocs/op       │
Encode/unsigned/big/1-8                  1.000 ± 0%
Encode/unsigned/big/4-8                  1.000 ± 0%
Encode/unsigned/big/8-8                  1.000 ± 0%
Encode/unsigned/big/16-8                 1.000 ± 0%
Encode/unsigned/little/1-8               1.000 ± 0%
Encode/unsigned/little/4-8               1.000 ± 0%
Encode/unsigned/little/8-8               1.000 ± 0%
Encode/unsigned/little/16-8              2.000 ± 0%
Encode/signed/big/8-8                    1.000 ± 0%
Encode/signed/little/8-8                 1.000 ± 0%
Decode/unsigned/big/1-8                  2.000 ± 0%
Decode/unsigned/big/4-8                  2.000 ± 0%
Decode/unsigned/big/8-8                  2.000 ± 0%
Decode/unsigned/big/16-8                 2.000 ± 0%
Decode/unsigned/little/1-8               2.000 ± 0%
Decode/unsigned/little/4-8               2.000 ± 0%
Decode/unsigned/little/8-8               2.000 ± 0%
Decode/unsigned/little/16-8              3.000 ± 0%
Decode/signed/big/8-8                    2.000 ± 0%
Decode/signed/little/8-8                 2.000 ± 0%
geomean                                  1.494

pkg: github.com/thebagchi/lark/v1/runtime/plugin/flow
                  │ 2026-11-01-134827.txt │
                  │        sec/op         │
Retry_EmptyBody-8            16.29µ ± 10%

                  │ 2026-11-01-134827.txt │
                  │         B/op          │
Retry_EmptyBody-8            4.016Ki ± 0%

                  │ 2026-11-01-134827.txt │
                  │       allocs/op       │
Retry_EmptyBody-8              58.00 ± 0%

pkg: github.com/thebagchi/lark/v1/runtime/plugin/hash
               │ 2026-11-01-134827.txt │
               │        sec/op         │
SHA256/16-8                183.8n ± 2%
SHA256/1024-8              636.9n ± 2%
SHA256/65536-8             30.41µ ± 2%
geomean                    1.527µ

               │ 2026-11-01-134827.txt │
               │         B/op          │
SHA256/16-8                 240.0 ± 0%
SHA256/1024-8               240.0 ± 0%
SHA256/65536-8              240.0 ± 0%
geomean                     240.0

               │ 2026-11-01-134827.txt │
               │       allocs/op       │
SHA256/16-8                 4.000 ± 0%
SHA256/1024-8               4.000 ± 0%
SHA256/65536-8              4.000 ± 0%
geomean                     4.000

               │ 2026-11-01-134827.txt │
               │          B/s          │
SHA256/16-8               83.00Mi ± 2%
SHA256/1024-8             1.497Gi ± 2%
SHA256/65536-8            2.007Gi ± 2%
geomean                   639.6Mi

pkg: github.com/thebagchi/lark/v1/runtime/plugin/state
                              │ 2026-11-01-134827.txt │
                              │        sec/op         │
Contend_TwoThreadsOnOneName-8             100.8m ± 0%
State_SetGet-8                            410.1µ ± 2%
State_Threads-8                           990.9µ ± 4%
geomean                                   3.447m

                              │ 2026-11-01-134827.txt │
                              │         B/op          │
Contend_TwoThreadsOnOneName-8            10.58Ki ± 3%
State_SetGet-8                           51.23Ki ± 0%
State_Threads-8                          93.50Ki ± 0%
geomean                                  37.01Ki

                              │ 2026-11-01-134827.txt │
                              │       allocs/op       │
Contend_TwoThreadsOnOneName-8              153.0 ± 1%
State_SetGet-8                            2.056k ± 0%
State_Threads-8                           2.246k ± 0%
geomean                                    890.7

                              │ 2026-11-01-134827.txt │
                              │         ms/op         │
Contend_TwoThreadsOnOneName-8              100.5 ± 0%
```
