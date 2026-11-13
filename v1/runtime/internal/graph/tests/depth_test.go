package tests

import (
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	DEPTH_CHAIN = 257
)

// _Chain is a Loader that serves a linear chain of modules: module 0 loads
// module 1, module 1 loads module 2, and so on up to DEPTH_CHAIN - 1. The
// last module defines a function so that the flow has something to list.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Chain struct {
	depth int
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (c *_Chain) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the source of the module at name: a load of the next module
// in the chain, or a function when the chain ends.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (c *_Chain) Load(name string) ([]byte, error) {
	var idx int

	_, err := fmt.Sscanf(
		name,
		"m%d.star",
		&idx,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"unexpected module name %s",
			name,
		)
	}

	if idx >= c.depth-1 {
		return []byte("def leaf():\n    pass\n"), nil
	}

	return []byte(fmt.Sprintf(
		"load(\"m%d.star\", \"leaf\")\n\ndef leaf():\n    pass\n",
		idx+1,
	)), nil
}

// TestRegression_Depth proves a chain longer than MAX_DEPTH returns an error
// instead of overflowing the stack. The test process must stay up.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_Depth(t *testing.T) {
	var (
		entry = "entry.star"
		text  = []byte(
			"load(\"m0.star\", \"leaf\")\n\n" +
				"def main():\n    leaf()\n",
		)
		loader = &_Chain{
			depth: DEPTH_CHAIN,
		}
		source = &common.Source{
			Entry:  entry,
			Text:   text,
			Loader: loader,
		}
	)

	_, err := graph.Of(source)
	if !errors.Is(
		err,
		spelling.ErrDepth,
	) {
		t.Fatalf(
			"got %v, want ErrDepth",
			err,
		)
	}

	t.Logf(
		"refused: %v",
		err,
	)
}
