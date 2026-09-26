package observe_test

import (
	_ "github.com/thebagchi/lark/v1/runtime/plugin/core"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
)

// The runtime's own builtins and the flow wrappers arrive by import, as they
// do for a host. Nothing else has to be registered by hand.
