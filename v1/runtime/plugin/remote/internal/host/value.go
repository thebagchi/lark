package host

import (
	"fmt"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Starlark is a protobuf value as Starlark sees it.
//
// The conversion is shared with the args plugin. A whole number arrives as an
// int, and a failure carries unpack.ErrConvert.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-11-02 11:24: converts with unpack.FromProto
//   - 2026-11-02 11:34: wraps a refusal with _Refused
//   - 2026-11-04 21:49: wraps the refusal itself, _Refused being gone (C59)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: wraps wire.ErrValue, the sentinel having moved (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Starlark(given *structpb.Value) (starlark.Value, error) {
	converted, err := unpack.FromProto(given)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot be put on the wire: %w",
			err,
		)
	}

	return converted, nil
}
