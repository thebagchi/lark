# Benchmark — binary

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-28 21:58 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^(BenchmarkEncode|BenchmarkDecode)$' -benchmem -count=6 ./v1/runtime/plugin/codec/binary
```

The name of each sub-benchmark is the count of data bytes.
For `BenchmarkEncode`, MB/s counts the data bytes, which are the input.
For `BenchmarkDecode`, MB/s counts the characters of the text, which are the input.
The text has eight characters for each data byte.
Thus, do not compare the MB/s of the two benchmarks.

### BenchmarkEncode/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 52.51 | 304.69 | 128 | 1 |
| 2 | 52.33 | 305.75 | 128 | 1 |
| 3 | 53.49 | 299.12 | 128 | 1 |
| 4 | 53.00 | 301.88 | 128 | 1 |
| 5 | 54.52 | 293.45 | 128 | 1 |
| 6 | 53.19 | 300.83 | 128 | 1 |

### BenchmarkEncode/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 2435 | 420.45 | 8192 | 1 |
| 2 | 2379 | 430.51 | 8192 | 1 |
| 3 | 2385 | 429.27 | 8192 | 1 |
| 4 | 2384 | 429.55 | 8192 | 1 |
| 5 | 2413 | 424.33 | 8192 | 1 |
| 6 | 2397 | 427.15 | 8192 | 1 |

### BenchmarkEncode/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 115185 | 568.96 | 524299 | 1 |
| 2 | 115524 | 567.29 | 524300 | 1 |
| 3 | 116934 | 560.45 | 524299 | 1 |
| 4 | 115736 | 566.25 | 524299 | 1 |
| 5 | 121538 | 539.22 | 524299 | 1 |
| 6 | 119054 | 550.47 | 524301 | 1 |

### BenchmarkDecode/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 40.66 | 3148.36 | 16 | 1 |
| 2 | 40.89 | 3130.55 | 16 | 1 |
| 3 | 40.10 | 3192.05 | 16 | 1 |
| 4 | 41.48 | 3085.87 | 16 | 1 |
| 5 | 40.64 | 3149.40 | 16 | 1 |
| 6 | 39.89 | 3208.46 | 16 | 1 |

### BenchmarkDecode/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 1496 | 5476.96 | 1024 | 1 |
| 2 | 1501 | 5456.26 | 1024 | 1 |
| 3 | 1485 | 5516.27 | 1024 | 1 |
| 4 | 1534 | 5341.62 | 1024 | 1 |
| 5 | 1499 | 5465.62 | 1024 | 1 |
| 6 | 1501 | 5456.35 | 1024 | 1 |

### BenchmarkDecode/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 95816 | 5471.83 | 65536 | 1 |
| 2 | 97729 | 5364.73 | 65536 | 1 |
| 3 | 97782 | 5361.81 | 65536 | 1 |
| 4 | 100459 | 5218.91 | 65536 | 1 |
| 5 | 107369 | 4883.04 | 65536 | 1 |
| 6 | 103599 | 5060.76 | 65536 | 1 |
