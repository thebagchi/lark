package tests

import (
	"strconv"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
)

const (
	// WIDE is the count of members in the wide dict of the find_key benchmark.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	WIDE = 4096

	// ROWS is the count of dicts in the list of the find_key benchmark.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ROWS = 1000

	// COLUMNS is the count of members in each of those dicts.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	COLUMNS = 10

	// NESTING is how many lists the nested document of the benchmark has.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NESTING = 20

	// DEPTH is how many levels of dicts the tree document of the benchmark has.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DEPTH = 4

	// FANOUT is the count of members in each dict of that tree.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FANOUT = 8

	// RECORDS is the count of dicts in the list of the records case.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RECORDS = 5000
)

// BenchmarkFindKey_Miss measures find_key on four documents that do not hold
// the key, so the whole document is walked: a dict of WIDE numbers, a list of
// ROWS dicts of COLUMNS numbers, lists nested NESTING deep, and a tree of dicts
// DEPTH levels deep with FANOUT members each, and a list of RECORDS dicts that
// each hold a number, a string and a list. The walk
// carries a trail, so allocs/op shows the cost of the cycle and depth check.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26), from the scratch benchmark of
//     poc/research/q2_nocrash
//   - 2026-11-02 17:30: add the tree case (C36c)
//   - 2026-11-02 20:46: add the records case (C36c)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkFindKey_Miss(b *testing.B) {
	cases := []struct {
		name string
		doc  starlark.Value
	}{
		{
			name: "dict of " + strconv.Itoa(WIDE) + " numbers",
			doc:  _Wide(b),
		},
		{
			name: "list of " + strconv.Itoa(ROWS) + " dicts",
			doc:  _Rows(b),
		},
		{
			name: "list nested " + strconv.Itoa(NESTING),
			doc:  _Nested(),
		},
		{
			name: "tree " + strconv.Itoa(DEPTH) + "x" + strconv.Itoa(FANOUT),
			doc: _Tree(
				b,
				DEPTH,
			),
		},
		{
			name: "list of " + strconv.Itoa(RECORDS) + " records",
			doc:  _Records(b),
		},
	}

	for _, tc := range cases {
		b.Run(
			tc.name,
			func(b *testing.B) {
				_Call(
					b,
					_Builtin(
						b,
						spec.FIND,
					),
					starlark.Tuple{
						tc.doc,
						starlark.String("absent"),
					},
				)
			},
		)
	}
}

// _Wide is a dict of WIDE members whose values are numbers.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Wide(b *testing.B) *starlark.Dict {
	b.Helper()

	dict := starlark.NewDict(WIDE)

	for idx := range WIDE {
		err := dict.SetKey(
			starlark.String("k"+strconv.Itoa(idx)),
			starlark.MakeInt(idx),
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	return dict
}

// _Rows is a list of ROWS dicts, each of COLUMNS members.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Rows(b *testing.B) *starlark.List {
	b.Helper()

	rows := make(
		[]starlark.Value,
		0,
		ROWS,
	)

	for range ROWS {
		row := starlark.NewDict(COLUMNS)

		for idx := range COLUMNS {
			err := row.SetKey(
				starlark.String("c"+strconv.Itoa(idx)),
				starlark.MakeInt(idx),
			)
			if err != nil {
				b.Fatal(err)
			}
		}

		rows = append(
			rows,
			row,
		)
	}

	return starlark.NewList(rows)
}

// _Nested is a list that holds a list, NESTING times.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Nested() *starlark.List {
	nested := starlark.NewList(nil)

	for range NESTING - 1 {
		nested = starlark.NewList([]starlark.Value{nested})
	}

	return nested
}

// _Tree is a dict of FANOUT members named member_0 up. Each member holds a tree
// of one level less, and the last level holds numbers.
//
// Revisions:
//   - 2026-11-02 17:30: initial creation (C36c), from _Tree of poc/research/simd_json
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Tree(
	b *testing.B,
	depth int,
) starlark.Value {
	b.Helper()

	if depth == 0 {
		return starlark.MakeInt(7)
	}

	dict := starlark.NewDict(FANOUT)

	for idx := range FANOUT {
		err := dict.SetKey(
			starlark.String("member_"+strconv.Itoa(idx)),
			_Tree(
				b,
				depth-1,
			),
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	return dict
}

// _Records is a list of RECORDS dicts. Each dict holds id, name and tags: a
// number, a string and a list of one string.
//
// Revisions:
//   - 2026-11-02 20:46: initial creation (C36c), from BenchmarkFindKey_Miss_Lists of
//     poc/research/simd_json
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Records(b *testing.B) *starlark.List {
	b.Helper()

	records := make(
		[]starlark.Value,
		0,
		RECORDS,
	)

	for idx := range RECORDS {
		record := starlark.NewDict(3)

		for key, value := range map[string]starlark.Value{
			"id":   starlark.MakeInt(idx),
			"name": starlark.String("x"),
			"tags": starlark.NewList([]starlark.Value{
				starlark.String("a"),
			}),
		} {
			err := record.SetKey(
				starlark.String(key),
				value,
			)
			if err != nil {
				b.Fatal(err)
			}
		}

		records = append(
			records,
			record,
		)
	}

	return starlark.NewList(records)
}
