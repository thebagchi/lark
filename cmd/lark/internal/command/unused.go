package command

import (
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/thebagchi/lark/internal/declared"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Unused is the run option that hands a run only the -args keys its script
// declares, after one line on standard error for each key it does not. It is
// nil when every key is declared, when -args was not given, when a bundle is
// run, or when the names cannot be read.
//
// The library refuses a key no arg() declares (Q170). The command warns and
// runs (Q136), so it drops those keys before the run is handed them. The
// names come from the merged script, or the script a flow generates for a
// FILE of .json (Q186, Q193, Q308). A bundle carries no flow here, so
// the library's refusal applies to it and the run fails with its error.
//
// The merge reads the script once more, and only when -args is given.
//
// Revisions:
//   - 2026-11-05 19:20: initial creation (C71, Q136, Q186)
//   - 2026-11-05 22:10: takes the compile options, and reads names from the
//     merge rather than Derive's flow (Q193)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
//   - 2026-11-10 11:45: a flow is a FILE of .json, not -flow (C119, Q308)
//   - 2026-11-13 13:12: reads the arguments with Arguments, so a number keeps its text (C132,
//     Q437)
func _Unused(
	asked *Asked,
	src []byte,
	opts []common.CompileOption,
) []common.RunOption {
	if asked.Supplied == "" || asked.Noun == BUNDLE {
		return nil
	}

	declared, ok := _Names(
		asked,
		src,
		opts,
	)
	if !ok {
		return nil
	}

	args, err := Arguments(asked.Supplied)
	if err != nil {
		return nil
	}

	var dropped bool

	for _, key := range slices.Sorted(maps.Keys(args)) {
		if slices.Contains(
			declared,
			key,
		) {
			continue
		}

		_Warn(
			key,
			declared,
		)
		delete(
			args,
			key,
		)

		dropped = true
	}

	if !dropped {
		return nil
	}

	return []common.RunOption{runtime.WithArgs(args)}
}

// _Names is the names every top-level arg() call of the script declares, and
// whether they could be read. They are read from the text a compile runs: the
// merge of the script and its modules, or the script a flow generates, so the
// names are the ones the library checks a run against (Q164, Q193). A script
// the merge refuses has no names to check against, and the run reports its
// own errors.
//
// Revisions:
//   - 2026-11-05 19:20: initial creation (C71)
//   - 2026-11-05 22:10: reads every top-level arg() call from the merged
//     script, since Derive lists only the assigned ones (Q193)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Names(
	asked *Asked,
	src []byte,
	opts []common.CompileOption,
) ([]string, bool) {
	var (
		entry = asked.Path
		text  []byte
		err   error
	)

	if asked.Noun == SCRIPT {
		text, err = _Merged(
			asked.Path,
			src,
			opts,
		)
	} else {
		entry += GENERATED
		text, err = Generated(
			asked.Path,
			src,
		)
	}

	if err != nil {
		return nil, false
	}

	names, err := declared.Of(
		entry,
		text,
	)

	return names, err == nil
}

// _Merged is the script and the modules it loads as the one text a compile
// runs.
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71, Q193)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
func _Merged(
	path string,
	src []byte,
	opts []common.CompileOption,
) ([]byte, error) {
	merged, err := runtime.Merge(
		&common.Source{
			Entry: path,
			Text:  src,
		},
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return merged.Text, nil
}

// _Warn writes the line for one key no arg() declares: the key quoted and cut
// (Q4), and the declared name nearest to it when one is close (Q158).
//
// Revisions:
//   - 2026-11-05 19:20: initial creation (C71)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Warn(
	key string,
	declared []string,
) {
	var hint string

	near := spelling.Nearest(
		key,
		declared,
	)
	if near != "" {
		hint = fmt.Sprintf(
			" (did you mean %s?)",
			unpack.QuotePrefix(near),
		)
	}

	_, _ = fmt.Fprintf(
		os.Stderr,
		"lark: argument %s is not declared by the script%s\n",
		unpack.QuotePrefix(key),
		hint,
	)
}
