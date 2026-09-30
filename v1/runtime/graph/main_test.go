package graph_test

import (
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/args"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/time"
)

// A blank import is the whole of enabling a plugin: each registers itself when
// it is imported, and nothing else in this package's tests would pull it in.
//
// flow is needed because this package generates calls to repeat, retry, timeout
// and sleep, and a test that compiles what it generated has to have those names
// in scope. args is needed because a thread can be handed a run's argument, and
// a test that runs one declares it. The rest are needed because the round trip
// runs every sample, and the samples are what the plugins are documented by. A
// host does the same thing for the same reason - the facade registers the
// scheduler's own builtins and no more.

// _Valued is an argument passing value, as a test writes one.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Valued(value *structpb.Value) *workflowpb.Parameters {
	return &workflowpb.Parameters{Param: &workflowpb.Parameters_Value{Value: value}}
}

// _Parameter is an argument passing whatever the name holds where the call is
// made.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Parameter(name string) *workflowpb.Parameters {
	return &workflowpb.Parameters{Param: &workflowpb.Parameters_Parameter{Parameter: name}}
}
