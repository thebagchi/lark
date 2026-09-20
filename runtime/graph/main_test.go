package graph_test

import (
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/json"
	_ "github.com/thebagchi/lark/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/runtime/plugin/math"
	_ "github.com/thebagchi/lark/runtime/plugin/state"
	_ "github.com/thebagchi/lark/runtime/plugin/time"
)

// A blank import is the whole of enabling a plugin: each registers itself when
// it is imported, and nothing else in this package's tests would pull it in.
//
// flow is needed because this package generates calls to repeat, retry, timeout
// and sleep, and a test that compiles what it generated has to have those names
// in scope. The rest are needed because the round trip runs every sample, and
// the samples are what the plugins are documented by. A host does the same
// thing for the same reason - the facade registers the scheduler's own builtins
// and no more.
