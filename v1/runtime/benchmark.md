# Benchmark — runtime

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.10.
Date: 2026-10-28 21:58 IST.

## Baseline

Command:

```
go test -run='^$' -bench='^(BenchmarkRun_Return|BenchmarkSpawn_Join|BenchmarkCompile_Functions)$' -benchmem -count=6 ./v1/runtime
```

`BenchmarkRun_Return` runs a script with a `main` that returns 1.
`BenchmarkSpawn_Join` runs a script with a `main` that spawns 1, 16, or 256 threads and then joins them.
`BenchmarkCompile_Functions` compiles a source of 200 small functions and a `main`.
It does not run the source.

### BenchmarkRun_Return

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 14963 | 5224 | 65 |
| 2 | 15046 | 5224 | 65 |
| 3 | 14841 | 5224 | 65 |
| 4 | 15059 | 5224 | 65 |
| 5 | 14900 | 5224 | 65 |
| 6 | 14825 | 5224 | 65 |

### BenchmarkSpawn_Join/1

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 24315 | 8339 | 120 |
| 2 | 25640 | 8339 | 120 |
| 3 | 25938 | 8339 | 120 |
| 4 | 24969 | 8339 | 120 |
| 5 | 23863 | 8339 | 120 |
| 6 | 24904 | 8339 | 120 |

### BenchmarkSpawn_Join/16

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 91201 | 45339 | 608 |
| 2 | 90686 | 45305 | 608 |
| 3 | 91889 | 45330 | 608 |
| 4 | 91422 | 45296 | 608 |
| 5 | 90887 | 45319 | 608 |
| 6 | 90640 | 45282 | 608 |

### BenchmarkSpawn_Join/256

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 857227 | 599057 | 8074 |
| 2 | 861620 | 598544 | 8074 |
| 3 | 902443 | 599845 | 8074 |
| 4 | 872724 | 599620 | 8074 |
| 5 | 874630 | 598762 | 8074 |
| 6 | 874932 | 598437 | 8073 |

### BenchmarkCompile_Functions

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 2452801 | 1644873 | 26188 |
| 2 | 2359939 | 1644873 | 26188 |
| 3 | 2391038 | 1644871 | 26188 |
| 4 | 2369860 | 1644881 | 26188 |
| 5 | 2371680 | 1644882 | 26188 |
| 6 | 2338921 | 1644882 | 26188 |
