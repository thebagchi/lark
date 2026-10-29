# Benchmark — base64

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-29 00:57 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^BenchmarkEncode$' -benchmem -count=6 ./v1/runtime/plugin/base64
```

Each benchmark calls the builtin directly, through `CallInternal`.
A script does not run.
Thus, the numbers do not include the cost of a script.
The name of each sub-benchmark is the count of data bytes.

The input is a `bytes` value.
MB/s counts the data bytes, which are the input.

### BenchmarkEncode/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 80.73 | 198.18 | 72 | 4 |
| 2 | 84.78 | 188.72 | 72 | 4 |
| 3 | 93.60 | 170.94 | 72 | 4 |
| 4 | 106.3 | 150.51 | 72 | 4 |
| 5 | 85.15 | 187.91 | 72 | 4 |
| 6 | 83.49 | 191.65 | 72 | 4 |

### BenchmarkEncode/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 1228 | 833.93 | 3872 | 5 |
| 2 | 1287 | 795.74 | 3872 | 5 |
| 3 | 1256 | 815.30 | 3872 | 5 |
| 4 | 1864 | 549.27 | 3872 | 5 |
| 5 | 1407 | 727.54 | 3872 | 5 |
| 6 | 1421 | 720.68 | 3872 | 5 |

### BenchmarkEncode/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 79631 | 823.00 | 245793 | 5 |
| 2 | 72625 | 902.39 | 245793 | 5 |
| 3 | 77292 | 847.90 | 245793 | 5 |
| 4 | 73960 | 886.10 | 245793 | 5 |
| 5 | 77795 | 842.42 | 245793 | 5 |
| 6 | 73810 | 887.90 | 245793 | 5 |
