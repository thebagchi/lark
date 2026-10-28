# Benchmark — flow

Machine: 12th Gen Intel Core i7-12800H, linux/amd64, WSL2.
Go: 1.26.7.
Date: 2026-10-24 17:52 IST.

## Baseline

Command:

```
go test -run='^$' -bench=BenchmarkRetry_EmptyBody -benchmem -count=6 ./v1/runtime/plugin/flow
```

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 33964 | 7736 | 107 |
| 2 | 31807 | 7736 | 107 |
| 3 | 29666 | 7736 | 107 |
| 4 | 29002 | 7736 | 107 |
| 5 | 29236 | 7736 | 107 |
| 6 | 29615 | 7736 | 107 |
