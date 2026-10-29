# Benchmark — hex

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-29 00:57 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^(BenchmarkFromBytes|BenchmarkToBytes)$' -benchmem -count=6 ./v1/runtime/plugin/codec/hex
```

Each benchmark calls the builtin directly, through `CallInternal`.
A script does not run.
Thus, the numbers do not include the cost of a script.
The name of each sub-benchmark is the count of data bytes.

For `BenchmarkFromBytes`, the input is a `bytes` value.
MB/s counts the data bytes, which are the input.
For `BenchmarkToBytes`, the input is a `str` value of hex.
MB/s counts the characters of the text, which are the input.
The text has two characters for each data byte.
Thus, do not compare the MB/s of the two benchmarks.

### BenchmarkFromBytes/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 157.2 | 101.76 | 104 | 5 |
| 2 | 157.4 | 101.67 | 104 | 5 |
| 3 | 157.8 | 101.38 | 104 | 5 |
| 4 | 158.0 | 101.26 | 104 | 5 |
| 5 | 158.3 | 101.07 | 104 | 5 |
| 6 | 158.1 | 101.20 | 104 | 5 |

### BenchmarkFromBytes/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 1762 | 581.28 | 3130 | 5 |
| 2 | 1784 | 573.84 | 3130 | 5 |
| 3 | 1714 | 597.59 | 3130 | 5 |
| 4 | 1669 | 613.57 | 3130 | 5 |
| 5 | 1659 | 617.37 | 3130 | 5 |
| 6 | 1715 | 597.07 | 3130 | 5 |

### BenchmarkFromBytes/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 174307 | 375.98 | 711784 | 29 |
| 2 | 217485 | 301.34 | 711780 | 29 |
| 3 | 271800 | 241.12 | 711780 | 29 |
| 4 | 220102 | 297.75 | 711789 | 29 |
| 5 | 200700 | 326.54 | 711791 | 29 |
| 6 | 203915 | 321.39 | 711791 | 29 |

### BenchmarkToBytes/16

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 89.29 | 358.38 | 64 | 4 |
| 2 | 88.80 | 360.38 | 64 | 4 |
| 3 | 88.61 | 361.13 | 64 | 4 |
| 4 | 89.18 | 358.82 | 64 | 4 |
| 5 | 87.95 | 363.86 | 64 | 4 |
| 6 | 85.81 | 372.90 | 64 | 4 |

### BenchmarkToBytes/1024

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 1136 | 1803.61 | 2080 | 4 |
| 2 | 1109 | 1846.13 | 2080 | 4 |
| 3 | 1109 | 1847.09 | 2080 | 4 |
| 4 | 1120 | 1829.18 | 2080 | 4 |
| 5 | 1100 | 1861.44 | 2080 | 4 |
| 6 | 1127 | 1816.52 | 2080 | 4 |

### BenchmarkToBytes/65536

| Run | ns/op | MB/s | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 61643 | 2126.32 | 131104 | 4 |
| 2 | 61344 | 2136.68 | 131104 | 4 |
| 3 | 61527 | 2130.33 | 131104 | 4 |
| 4 | 59955 | 2186.19 | 131104 | 4 |
| 5 | 60528 | 2165.49 | 131104 | 4 |
| 6 | 62705 | 2090.31 | 131104 | 4 |
