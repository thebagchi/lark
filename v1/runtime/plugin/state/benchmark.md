# Benchmark — state

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.7.
Date: 2026-10-24 17:52 IST.

## Baseline

Command:

```
go test -run='^$' -bench=BenchmarkContend_TwoThreadsOnOneName -benchmem -count=6 ./v1/runtime/plugin/state
```

| Run | ns/op | ms/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 1 | 101001031 | 100 | 13743 | 185 |
| 2 | 101224911 | 100 | 13490 | 184 |
| 3 | 100827416 | 100 | 13602 | 184 |
| 4 | 100953298 | 100 | 13735 | 185 |
| 5 | 100942801 | 100 | 13552 | 183 |
| 6 | 100897659 | 100 | 13480 | 184 |
