# Benchmark — hash

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-29 00:57 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^BenchmarkSHA256$' -benchmem -count=6 ./v1/runtime/plugin/hash
```

Each benchmark calls the builtin directly, through `CallInternal`.
A script does not run.
Thus, the numbers do not include the cost of a script.
The name of each sub-benchmark is the count of data bytes.

The input is a `bytes` value.
MB/s counts the data bytes, which are the input.
The output is the digest as upper-case hex.

### BenchmarkSHA256/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 452.6 | 35.35 | 296 | 7 |
| 2 | 327.8 | 48.81 | 296 | 7 |
| 3 | 324.9 | 49.24 | 296 | 7 |
| 4 | 388.8 | 41.15 | 296 | 7 |
| 5 | 364.9 | 43.85 | 296 | 7 |
| 6 | 517.8 | 30.90 | 296 | 7 |

### BenchmarkSHA256/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 1200 | 853.66 | 1304 | 7 |
| 2 | 1223 | 837.49 | 1304 | 7 |
| 3 | 1225 | 836.22 | 1304 | 7 |
| 4 | 1086 | 942.80 | 1304 | 7 |
| 5 | 1021 | 1002.53 | 1304 | 7 |
| 6 | 1023 | 1000.68 | 1304 | 7 |

### BenchmarkSHA256/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 43656 | 1501.20 | 65846 | 7 |
| 2 | 46753 | 1401.75 | 65845 | 7 |
| 3 | 42814 | 1530.72 | 65846 | 7 |
| 4 | 41596 | 1575.52 | 65846 | 7 |
| 5 | 41958 | 1561.96 | 65846 | 7 |
| 6 | 41517 | 1578.52 | 65846 | 7 |
