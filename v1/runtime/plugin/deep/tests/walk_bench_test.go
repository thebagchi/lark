package tests

import (
	"context"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

const (
	// FLAT is the count of numbers in the flat list of the walk benchmarks.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FLAT = 1000

	// DICTS is the count of dicts in the list of dicts.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DICTS = 100

	// MEMBERS is the count of members of each of those dicts.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MEMBERS = 10

	// LEVELS is how many lists the nested value has.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LEVELS = 20
)

// _WalkCases are the values the walk benchmarks use: a flat list, a list of
// dicts, a nested list, and a scalar.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _WalkCases(b *testing.B) []struct {
	name  string
	value starlark.Value
} {
	b.Helper()

	flat := make(
		[]starlark.Value,
		FLAT,
	)
	for idx := range flat {
		flat[idx] = starlark.MakeInt(idx)
	}

	dicts := make(
		[]starlark.Value,
		DICTS,
	)

	for idx := range dicts {
		dict := starlark.NewDict(MEMBERS)

		for member := range MEMBERS {
			err := dict.SetKey(
				starlark.MakeInt(member),
				starlark.MakeInt(idx),
			)
			if err != nil {
				b.Fatal(err)
			}
		}

		dicts[idx] = dict
	}

	var nested starlark.Value = starlark.NewList(nil)

	for range LEVELS - 1 {
		nested = starlark.NewList([]starlark.Value{nested})
	}

	return []struct {
		name  string
		value starlark.Value
	}{
		{
			"list of 1000 numbers",
			starlark.NewList(flat),
		},
		{
			"list of 100 dicts of 10",
			starlark.NewList(dicts),
		},
		{
			"list nested 20",
			nested,
		},
		{
			"a scalar",
			starlark.MakeInt(1),
		},
	}
}

// BenchmarkWalk_Copy measures Copy on the walk cases. allocs/op shows the
// cost of the trail, which is made once for a container and not for a scalar.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkWalk_Copy(b *testing.B) {
	for _, tc := range _WalkCases(b) {
		b.Run(
			tc.name,
			func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					_, err := deep.Copy(
						context.Background(),
						tc.value,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkWalk_IsData measures IsData on the walk cases.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkWalk_IsData(b *testing.B) {
	for _, tc := range _WalkCases(b) {
		b.Run(
			tc.name,
			func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					_, _, err := deep.IsData(
						context.Background(),
						tc.value,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
