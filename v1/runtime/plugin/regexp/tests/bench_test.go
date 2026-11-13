package tests

import (
	"fmt"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/regexp/internal/spec"
)

const (
	// BENCH is the name of the thread that each goroutine calls on.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH = "bench"

	// PATTERNS is the count of distinct patterns the goroutines share. It is
	// far under KEPT, so every call after the first round hits the cache.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PATTERNS = 8

	// SUBJECT is the text each call searches.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SUBJECT = "abc 12-34 def"
)

// BenchmarkRegexpParallel measures regexp.search from many goroutines at once,
// over PATTERNS patterns held by the plugin's cache, so the cost is the lock of
// the cache. Run it with -cpu 1,2,4,8 and compare the time per call across the
// counts: a lock that does not contend keeps it level.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from B6 of
//     poc/research/plugins_data.md
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkRegexpParallel(b *testing.B) {
	fn := _Builtin(
		b,
		spec.SEARCH,
	)

	var patterns = make(
		[]starlark.Tuple,
		PATTERNS,
	)

	for idx := range patterns {
		patterns[idx] = starlark.Tuple{
			starlark.String(fmt.Sprintf(
				`(\d+)-(\d+)%d?`,
				idx,
			)),
			starlark.String(SUBJECT),
		}
	}

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		var (
			thread = &starlark.Thread{
				Name: BENCH,
			}
			idx int
		)

		for pb.Next() {
			_, err := fn.CallInternal(
				thread,
				patterns[idx%PATTERNS],
				nil,
			)
			if err != nil {
				b.Error(err)

				return
			}

			idx++
		}
	})
}

// _Builtin is the member name of the regexp module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Builtin(
	b *testing.B,
	name string,
) *starlark.Builtin {
	b.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env[spec.NAME].(*starlarkstruct.Module)
	if !ok {
		b.Fatalf(
			"%s is not a module",
			spec.NAME,
		)
	}

	fn, ok := module.Members[name].(*starlark.Builtin)
	if !ok {
		b.Fatalf(
			"%s.%s is not a builtin",
			spec.NAME,
			name,
		)
	}

	return fn
}
