# Benchmark — integer

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-29 00:36 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^(BenchmarkEncode|BenchmarkDecode)$' -benchmem -count=6 ./v1/runtime/plugin/codec/integer
```

Each operation writes or reads one field.
`BenchmarkEncode` calls `Encode` or `EncodeSigned`, as `integer.to_bytes` does.
`BenchmarkDecode` calls `Decode` or `DecodeSigned`, as `integer.from_bytes` does.
The name of each sub-benchmark has three parts: the sign, the byte order, and the width in bytes.
Each signed field holds a negative number, so the complement is in the measurement.

### BenchmarkEncode/unsigned/big/1

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 25.97 | 1 | 1 |
| 2 | 25.67 | 1 | 1 |
| 3 | 26.06 | 1 | 1 |
| 4 | 26.61 | 1 | 1 |
| 5 | 27.01 | 1 | 1 |
| 6 | 26.54 | 1 | 1 |

### BenchmarkEncode/unsigned/big/4

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 26.84 | 4 | 1 |
| 2 | 27.50 | 4 | 1 |
| 3 | 26.56 | 4 | 1 |
| 4 | 27.43 | 4 | 1 |
| 5 | 27.25 | 4 | 1 |
| 6 | 27.93 | 4 | 1 |

### BenchmarkEncode/unsigned/big/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 29.18 | 8 | 1 |
| 2 | 28.66 | 8 | 1 |
| 3 | 28.46 | 8 | 1 |
| 4 | 28.70 | 8 | 1 |
| 5 | 28.05 | 8 | 1 |
| 6 | 28.68 | 8 | 1 |

### BenchmarkEncode/unsigned/big/16

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 37.64 | 16 | 1 |
| 2 | 37.51 | 16 | 1 |
| 3 | 37.59 | 16 | 1 |
| 4 | 36.36 | 16 | 1 |
| 5 | 36.19 | 16 | 1 |
| 6 | 37.54 | 16 | 1 |

### BenchmarkEncode/unsigned/little/1

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 46.88 | 16 | 2 |
| 2 | 45.88 | 16 | 2 |
| 3 | 45.88 | 16 | 2 |
| 4 | 48.40 | 16 | 2 |
| 5 | 48.40 | 16 | 2 |
| 6 | 47.27 | 16 | 2 |

### BenchmarkEncode/unsigned/little/4

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 49.40 | 16 | 2 |
| 2 | 49.74 | 16 | 2 |
| 3 | 48.94 | 16 | 2 |
| 4 | 47.86 | 16 | 2 |
| 5 | 47.36 | 16 | 2 |
| 6 | 47.86 | 16 | 2 |

### BenchmarkEncode/unsigned/little/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 43.78 | 16 | 2 |
| 2 | 44.84 | 16 | 2 |
| 3 | 45.18 | 16 | 2 |
| 4 | 46.37 | 16 | 2 |
| 5 | 46.28 | 16 | 2 |
| 6 | 45.22 | 16 | 2 |

### BenchmarkEncode/unsigned/little/16

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 61.76 | 32 | 2 |
| 2 | 60.39 | 32 | 2 |
| 3 | 59.85 | 32 | 2 |
| 4 | 60.82 | 32 | 2 |
| 5 | 59.70 | 32 | 2 |
| 6 | 61.18 | 32 | 2 |

### BenchmarkEncode/signed/big/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 43.34 | 16 | 2 |
| 2 | 44.36 | 16 | 2 |
| 3 | 43.88 | 16 | 2 |
| 4 | 44.48 | 16 | 2 |
| 5 | 74.54 | 16 | 2 |
| 6 | 62.24 | 16 | 2 |

### BenchmarkEncode/signed/little/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 79.25 | 24 | 3 |
| 2 | 71.76 | 24 | 3 |
| 3 | 65.82 | 24 | 3 |
| 4 | 65.05 | 24 | 3 |
| 5 | 65.01 | 24 | 3 |
| 6 | 65.09 | 24 | 3 |

### BenchmarkDecode/unsigned/big/1

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 36.18 | 40 | 2 |
| 2 | 36.36 | 40 | 2 |
| 3 | 36.07 | 40 | 2 |
| 4 | 36.66 | 40 | 2 |
| 5 | 36.84 | 40 | 2 |
| 6 | 36.26 | 40 | 2 |

### BenchmarkDecode/unsigned/big/4

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 38.13 | 40 | 2 |
| 2 | 38.96 | 40 | 2 |
| 3 | 37.78 | 40 | 2 |
| 4 | 39.12 | 40 | 2 |
| 5 | 39.25 | 40 | 2 |
| 6 | 39.46 | 40 | 2 |

### BenchmarkDecode/unsigned/big/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 37.21 | 40 | 2 |
| 2 | 35.23 | 40 | 2 |
| 3 | 34.62 | 40 | 2 |
| 4 | 36.56 | 40 | 2 |
| 5 | 35.47 | 40 | 2 |
| 6 | 35.47 | 40 | 2 |

### BenchmarkDecode/unsigned/big/16

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 47.14 | 80 | 2 |
| 2 | 48.34 | 80 | 2 |
| 3 | 48.42 | 80 | 2 |
| 4 | 48.68 | 80 | 2 |
| 5 | 48.20 | 80 | 2 |
| 6 | 51.14 | 80 | 2 |

### BenchmarkDecode/unsigned/little/1

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 54.70 | 48 | 3 |
| 2 | 55.03 | 48 | 3 |
| 3 | 54.31 | 48 | 3 |
| 4 | 55.40 | 48 | 3 |
| 5 | 55.00 | 48 | 3 |
| 6 | 56.50 | 48 | 3 |

### BenchmarkDecode/unsigned/little/4

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 58.78 | 48 | 3 |
| 2 | 57.89 | 48 | 3 |
| 3 | 58.71 | 48 | 3 |
| 4 | 103.0 | 48 | 3 |
| 5 | 57.37 | 48 | 3 |
| 6 | 69.59 | 48 | 3 |

### BenchmarkDecode/unsigned/little/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 58.74 | 48 | 3 |
| 2 | 57.53 | 48 | 3 |
| 3 | 57.10 | 48 | 3 |
| 4 | 62.67 | 48 | 3 |
| 5 | 60.11 | 48 | 3 |
| 6 | 60.91 | 48 | 3 |

### BenchmarkDecode/unsigned/little/16

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 76.02 | 96 | 3 |
| 2 | 74.95 | 96 | 3 |
| 3 | 77.33 | 96 | 3 |
| 4 | 79.45 | 96 | 3 |
| 5 | 75.01 | 96 | 3 |
| 6 | 76.19 | 96 | 3 |

### BenchmarkDecode/signed/big/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 90.45 | 96 | 4 |
| 2 | 92.48 | 96 | 4 |
| 3 | 85.70 | 96 | 4 |
| 4 | 83.34 | 96 | 4 |
| 5 | 81.98 | 96 | 4 |
| 6 | 81.00 | 96 | 4 |

### BenchmarkDecode/signed/little/8

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 100.1 | 104 | 5 |
| 2 | 105.1 | 104 | 5 |
| 3 | 103.1 | 104 | 5 |
| 4 | 102.8 | 104 | 5 |
| 5 | 102.4 | 104 | 5 |
| 6 | 101.2 | 104 | 5 |
