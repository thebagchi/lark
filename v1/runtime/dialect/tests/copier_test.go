package tests

// This file tests Copied, the copy that Compile compiles in place of a second
// parse. TestCompile_SameAsTwoParses compares the two on every script of the
// repository, and TestCopied_SharesNoNode checks the copy shares no node with
// the original. The second parse is built here from the steps that marked it.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from copier_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
)

const (
	ROOT       = "../../../.."
	SCRIPTS    = ".star"
	REPOSITORY = ".git"
)

// TestCompile_SameAsTwoParses compiles every script in the repository the way
// Compile does and the way it did before, with a second parse, and checks the
// two give the same program, or the same error.
//
// Every name is predeclared, so a script resolves without the environment it
// runs in; spawn is then the builtin, which is what makes the lambdas taken.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCompile_SameAsTwoParses(t *testing.T) {
	for _, path := range _Sources(t) {
		t.Run(
			path,
			func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}

				var (
					_, got, refused = _Whole(
						path,
						src,
						_Everything,
					)
					want, failed = _Twice(
						path,
						src,
						_Everything,
					)
				)

				_Agree(
					t,
					refused,
					failed,
				)

				if refused != nil {
					return
				}

				if !bytes.Equal(
					_Written(
						t,
						got,
					),
					_Written(
						t,
						want,
					),
				) {
					t.Fatal("the copy compiled to another program")
				}
			},
		)
	}
}

// TestCopied_SharesNoNode checks the copy of every script in the repository
// holds as many nodes as the tree it copies, and not one of them, so that
// resolving one tree writes nothing into the other.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopied_SharesNoNode(t *testing.T) {
	for _, path := range _Sources(t) {
		t.Run(
			path,
			func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}

				written, err := common.OPTIONS.Parse(
					path,
					src,
					0,
				)
				if err != nil {
					t.Skip("does not parse, so there is no tree to copy")
				}

				copied, err := dialect.Copied(written)
				if err != nil {
					t.Fatal(err)
				}

				var (
					nodes = _Nodes(written)
					fresh = _Nodes(copied)
				)

				if len(fresh) != len(nodes) {
					t.Fatalf(
						"copied %d nodes of %d",
						len(fresh),
						len(nodes),
					)
				}

				for node := range fresh {
					if nodes[node] {
						t.Fatalf(
							"the copy shares a %T",
							node,
						)
					}
				}
			},
		)
	}
}

// _Sources is every Starlark script in the repository: the samples and each
// package's test data.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Sources(t *testing.T) []string {
	t.Helper()

	var paths []string

	err := filepath.WalkDir(
		ROOT,
		func(
			path string,
			entry fs.DirEntry,
			err error,
		) error {
			if err != nil {
				return err
			}

			if entry.IsDir() && entry.Name() == REPOSITORY {
				return filepath.SkipDir
			}

			if filepath.Ext(path) == SCRIPTS {
				paths = append(
					paths,
					path,
				)
			}

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) == 0 {
		t.Fatal("found no script")
	}

	return paths
}

// _Everything predeclares every name.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Everything(string) bool {
	return true
}

// _Twice compiles src as Compile did before the copy: a second parse is
// marked and compiled, and the first is only resolved.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Twice(
	path string,
	src []byte,
	predeclared func(string) bool,
) (*starlark.Program, error) {
	written, err := common.OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, err
	}

	err = resolve.File(
		written,
		predeclared,
		starlark.Universe.Has,
	)
	if err != nil {
		return nil, err
	}

	compiled, err := common.OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, err
	}

	rewrite.Marked(
		compiled,
		written,
	)

	return starlark.FileProgram(
		compiled,
		predeclared,
	)
}

// _Agree fails unless both compiles failed with the same text or neither did.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Agree(
	t *testing.T,
	got error,
	want error,
) {
	t.Helper()

	if got == nil && want == nil {
		return
	}

	if got == nil || want == nil || got.Error() != want.Error() {
		t.Fatalf(
			"got %v, want %v",
			got,
			want,
		)
	}
}

// _Written is the program as it is saved.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Written(
	t *testing.T,
	program *starlark.Program,
) []byte {
	t.Helper()

	var buf bytes.Buffer

	err := program.Write(&buf)
	if err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// _Nodes is every node of a tree.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Nodes(file *syntax.File) map[syntax.Node]bool {
	nodes := make(map[syntax.Node]bool)

	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			if node != nil {
				nodes[node] = true
			}

			return true
		},
	)

	return nodes
}

// _Whole parses src, makes the two trees Compile takes, and compiles them.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from what Compile did itself
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Whole(
	path string,
	src []byte,
	predeclared func(string) bool,
) (*syntax.File, *starlark.Program, error) {
	written, err := common.OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, nil, err
	}

	compiled, err := dialect.Copied(written)
	if err != nil {
		return nil, nil, err
	}

	err = resolve.File(
		written,
		predeclared,
		starlark.Universe.Has,
	)
	if err != nil {
		return nil, nil, err
	}

	program, err := dialect.Compile(
		written,
		compiled,
		predeclared,
	)
	if err != nil {
		return nil, nil, err
	}

	return written, program, nil
}
