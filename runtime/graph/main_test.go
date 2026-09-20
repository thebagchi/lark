package graph_test

import (
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
)

// A blank import is the whole of enabling a plugin: flow registers itself when
// it is imported, and nothing else in this package's tests would pull it in.
//
// It is needed because this package generates calls to repeat, retry, timeout
// and sleep, and a test that compiles what it generated has to have those names
// in scope. A host does the same thing for the same reason - the facade
// registers the scheduler's own builtins and no more.
