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

## Baseline — one name, no contention and contention

Date: 2026-10-28 21:58 IST.
Go: 1.26.10.

Command:

```
go test -run='^$' -bench='^(BenchmarkState_SetGet|BenchmarkState_Threads)$' -benchmem -count=6 ./v1/runtime/plugin/state
```

`BenchmarkState_SetGet` runs one thread that does 1000 pairs of `state.set` and `state.get` on one name.
`BenchmarkState_Threads` runs 16 spawned threads. Each thread does 100 `state.set` on one name.
Then `main` joins the threads.

### BenchmarkState_SetGet

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 594216 | 222460 | 8079 |
| 2 | 606043 | 222457 | 8079 |
| 3 | 618202 | 222457 | 8079 |
| 4 | 596717 | 222457 | 8079 |
| 5 | 592345 | 222458 | 8079 |
| 6 | 592430 | 222457 | 8079 |

### BenchmarkState_Threads

| Run | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 | 1144136 | 269990 | 8720 |
| 2 | 1119834 | 269956 | 8720 |
| 3 | 1166464 | 269942 | 8720 |
| 4 | 1133847 | 269915 | 8720 |
| 5 | 1148115 | 269914 | 8720 |
| 6 | 1150536 | 269918 | 8720 |
