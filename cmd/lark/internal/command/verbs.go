package command

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// RUN is the verb that runs a script, a flow or a bundle. The verbs are the
	// first word of a command line (Q305, Q306), and one is required (Q307).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q305, Q306, Q307)
	RUN = "run"

	// COMPILE is the verb that writes a bundle (Q303).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q303)
	COMPILE = "compile"

	// CONVERT is the verb that prints the other form of a script or a flow (Q306).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q306)
	CONVERT = "convert"

	// VERSION is the verb that prints the versions of lark and of Go.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q305)
	VERSION = "version"

	// HELP is the verb that prints the help, or the help of one verb.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q305)
	HELP = "help"

	// FLOW is what a failure calls a flow that failed, as SCRIPT and BUNDLE are.
	//
	// Revisions:
	//   - 2026-11-10 16:54: initial creation, moved from cmd/lark/main.go into
	//     internal/command (C119, Q308)
	FLOW = "flow"

	// STDIN_ARG is the FILE that means standard input, and STDIN_NAME what a
	// position in that input is called: the name the Starlark tools give a
	// script read from there.
	//
	// Revisions:
	//   - 2026-11-05 00:32: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 16:54: moved from cmd/lark/main.go into internal/command (C119, Q308)
	STDIN_ARG = "-"

	// STDIN_NAME is what a position in the input read from standard input is
	// called.
	//
	// Revisions:
	//   - 2026-11-10 16:54: initial creation, split from the doc of STDIN_ARG in
	//     cmd/lark/main.go (C119, Q308)
	STDIN_NAME = "<stdin>"

	// EXT_BUNDLE is the extension of a bundle, and EXT_FLOW the extension of a
	// flow. Any other FILE is a script (Q308).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308)
	EXT_BUNDLE = ".bin"

	// EXT_FLOW is the extension of a flow.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308)
	EXT_FLOW = ".json"

	// REFUSAL starts the line the flag package writes for a flag it has no name
	// for. flag.(*FlagSet).parseOne writes it with the name after the dash.
	//
	// Revisions:
	//   - 2026-11-05 16:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 16:54: moved from cmd/lark/suggest.go into internal/command (C119,
	//     Q305)
	REFUSAL = "flag provided but not defined: -"

	// AS_SCRIPT is the value of -as for a script, the kind of standard input
	// when -as is not given. AS_FLOW and AS_BUNDLE are the other two kinds.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308; the values are proposed)
	AS_SCRIPT = SCRIPT

	// AS_FLOW is the value of -as for a flow.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308; the value is proposed)
	AS_FLOW = FLOW

	// AS_BUNDLE is the value of -as for a bundle.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308; the value is proposed)
	AS_BUNDLE = BUNDLE

	// ARGS_FLAG is the -args flag of run. Each flag belongs to the verbs the
	// help lists it under (Q305).
	//
	// Revisions:
	//   - 2026-11-10 16:54: initial creation, replacing the flag names of cmd/lark/main.go
	//     (C119, Q305)
	ARGS_FLAG = "args"

	// PLUGINS_FLAG is the -plugins flag of run and compile.
	//
	// Revisions:
	//   - 2026-11-10 16:54: initial creation, replacing the flag names of cmd/lark/main.go;
	//     run and compile only (C119, Q305, Q337)
	PLUGINS_FLAG = "plugins"

	// MEMORY_FLAG is the -memory flag of run.
	//
	// Revisions:
	//   - 2026-11-10 11:45: moved from cmd/lark (C119, Q305)
	MEMORY_FLAG = "memory"

	// HOLD_FLAG is the -hold flag of run.
	//
	// Revisions:
	//   - 2026-11-10 11:45: moved from cmd/lark (C119, Q305)
	HOLD_FLAG = "hold"

	// LOGS_FLAG is the -logs flag of run.
	//
	// Revisions:
	//   - 2026-11-10 11:45: moved from cmd/lark (C119, Q305)
	LOGS_FLAG = "logs"

	// GRAPH_FLAG is the -graph flag of run.
	//
	// Revisions:
	//   - 2026-11-10 11:45: moved from cmd/lark (C119, Q304, Q305)
	GRAPH_FLAG = "graph"

	// OUT_FLAG is the -o flag of compile (Q309).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q309)
	OUT_FLAG = "o"

	// AS_FLAG is the -as flag of run, compile and convert (Q308).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308)
	AS_FLAG = "as"
)

// Parse reads the command line args (without the program name) into an Asked,
// or says why it cannot be run.
//
// The first word is the verb (Q307). -h, -help and --help as the first word
// are the help, which is a person asking for it and not a verb that is wrong.
// Each verb has its own flags (Q305): a flag of another verb is a flag the
// command does not know. Flags may come before or after FILE. -h after a verb
// is the help of that verb.
//
// The Asked has the kind of the input (Noun), the path as the user gave it
// (File) and as a failure names it (Path). A refusal is returned for the first
// thing wrong, as Refusal orders them.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation, from _Flags and _Command of cmd/lark (C119, Q305, Q307)
//   - 2026-11-10 17:15: uses ErrVerb, formerly ErrNoVerb (C121, Q349)
func Parse(args []string) (*Asked, error) {
	if len(args) == 0 {
		return nil, ErrVerb
	}

	if slices.Contains(
		[]string{"-h", "-help", "--h", "--help"},
		args[0],
	) {
		return &Asked{
			Verb: HELP,
		}, nil
	}

	if !slices.Contains(
		_Verbs(),
		args[0],
	) {
		return nil, _Unknown(args[0])
	}

	var (
		asked = &Asked{
			Verb: args[0],
		}
		set = _Declared(asked)
	)

	files, err := _Gather(
		set,
		args[1:],
	)
	if errors.Is(
		err,
		flag.ErrHelp,
	) {
		return &Asked{
			Verb:  HELP,
			Topic: asked.Verb,
		}, nil
	}

	if err != nil {
		return nil, _Suggested(
			set,
			err,
		)
	}

	asked.Files = files

	err = _Placed(asked)
	if err != nil {
		return nil, err
	}

	return asked, nil
}

// _Verbs is the verbs, in the order the help lists them.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
func _Verbs() []string {
	return []string{RUN, COMPILE, CONVERT, VERSION, HELP}
}

// _Unknown is the refusal of a first word that is no verb: the word quoted and
// cut at 100 bytes (Q4), and the verb nearest to it when one is close (Q158).
// Leading dashes do not count, so -version points to version.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q307; the text is proposed)
func _Unknown(word string) error {
	var hint string

	near := spelling.Nearest(
		strings.TrimLeft(
			word,
			"-",
		),
		_Verbs(),
	)
	if near != "" {
		hint = " (did you mean " + near + "?)"
	}

	return fmt.Errorf(
		"lark: unknown verb %s%s",
		unpack.QuotePrefix(word),
		hint,
	)
}

// _Declared is the flags of the verb of asked, declared into asked.
//
// The flag package would print its errors and its usage by itself. Neither is
// wanted: Parse returns the error, and the help is the help text.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-01 12:00: full flag names, FLOW_FLAG for the flow input, no -m
//   - 2026-11-02 21:04: -graph and -changes
//   - 2026-11-02 22:32: -memory (C27)
//   - 2026-11-03 00:50: -hold (C42)
//   - 2026-11-03 23:57: declares each flag into the _Asked, with no struct of pointers (C57)
//   - 2026-11-05 08:07: the flag package reports through _Suggesting (C70)
//   - 2026-11-05 18:10: continues on error, with no usage of its own (C68)
//   - 2026-11-05 18:40: flags carry no usage text; help.txt is the one source (C68)
//   - 2026-11-10 11:32: reads Asked from command, which moved to internal/command (C120b, Q315)
//   - 2026-11-10 16:54: renamed from _Command, moved from cmd/lark/main.go into
//     internal/command/verbs.go, and declares the flags of one verb (C119, Q305)
func _Declared(asked *Asked) *flag.FlagSet {
	set := flag.NewFlagSet(
		"lark "+asked.Verb,
		flag.ContinueOnError,
	)
	set.SetOutput(io.Discard)
	set.Usage = func() {
		// Nothing: the help text is the usage.
	}

	_Texts(
		set,
		asked,
	)
	_Switches(
		set,
		asked,
	)

	return set
}

// _Texts declares into set the flags that take a value and that belong to the
// verb of asked.
//
// Revisions:
//   - 2026-11-04 00:05: initial creation, out of _Command (C57)
//   - 2026-11-05 18:40: no usage text per flag (C68)
//   - 2026-11-10 11:32: reads Asked from command, which moved to internal/command (C120b, Q315)
//   - 2026-11-10 16:54: moved from cmd/lark/main.go into internal/command/verbs.go;
//     declares the flags of one verb, -plugins for run and compile only (C119, Q305, Q337)
func _Texts(
	set *flag.FlagSet,
	asked *Asked,
) {
	files := []string{RUN, COMPILE, CONVERT}

	for _, item := range []struct {
		verbs []string
		into  *string
		name  string
	}{
		{
			[]string{RUN},
			&asked.Supplied,
			ARGS_FLAG,
		},
		{
			[]string{RUN, COMPILE},
			&asked.Plugins,
			PLUGINS_FLAG,
		},
		{
			[]string{RUN},
			&asked.Logs,
			LOGS_FLAG,
		},
		{
			[]string{RUN},
			&asked.Graph,
			GRAPH_FLAG,
		},
		{
			[]string{COMPILE},
			&asked.Out,
			OUT_FLAG,
		},
		{
			files,
			&asked.As,
			AS_FLAG,
		},
	} {
		if slices.Contains(
			item.verbs,
			asked.Verb,
		) {
			set.StringVar(
				item.into,
				item.name,
				"",
				"",
			)
		}
	}
}

// _Switches declares into set the flags that are on or off and that belong to
// the verb of asked.
//
// Revisions:
//   - 2026-11-04 00:05: initial creation, out of _Command (C57)
//   - 2026-11-04 22:52: -version (C67)
//   - 2026-11-05 18:40: no usage text per flag (C68)
//   - 2026-11-10 11:32: reads Asked from command, which moved to internal/command (C120b, Q315)
//   - 2026-11-10 16:54: moved from cmd/lark/main.go into internal/command/verbs.go;
//     declares the flags of one verb (C119, Q305)
func _Switches(
	set *flag.FlagSet,
	asked *Asked,
) {
	if asked.Verb != RUN {
		return
	}

	set.BoolVar(
		&asked.Memory,
		MEMORY_FLAG,
		false,
		"",
	)
	set.BoolVar(
		&asked.Hold,
		HOLD_FLAG,
		false,
		"",
	)
}

// _Gather reads args as flags and collects the words that are not flags, in
// order. The flag package stops at the first such word, so the rest is parsed
// again, and a flag may come after FILE.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation, from _Flags of cmd/lark (C119, Q305)
func _Gather(
	set *flag.FlagSet,
	args []string,
) ([]string, error) {
	var files []string

	err := set.Parse(args)
	for err == nil && set.NArg() > 0 {
		files = append(
			files,
			set.Arg(0),
		)

		err = set.Parse(set.Args()[1:])
	}

	return files, err
}

// _Suggested is err, with the one flag of set nearest to the name given when
// err is the refusal of a flag the verb does not have: "(did you mean -graph?)",
// in the words go.starlark.net uses for a name (Q158). Any other err is as it is.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-10 16:54: renamed from the method Write of _Suggesting, moved from
//     cmd/lark/suggest.go into internal/command/verbs.go, and a function over the error
//     (C119, Q305)
func _Suggested(
	set *flag.FlagSet,
	err error,
) error {
	name, found := strings.CutPrefix(
		err.Error(),
		REFUSAL,
	)
	if !found || strings.Contains(
		name,
		"\n",
	) {
		return err
	}

	var names []string

	set.VisitAll(func(f *flag.Flag) {
		names = append(
			names,
			f.Name,
		)
	})

	near := spelling.Nearest(
		name,
		names,
	)
	if near == "" {
		return err
	}

	return fmt.Errorf(
		"%w (did you mean -%s?)",
		err,
		near,
	)
}

// _Placed fills in what the words of asked say about the input: the FILE, the
// path a failure names, whether it is standard input, and the kind (Noun).
// The kind is by extension (Q308); standard input is a script unless -as says
// flow or bundle. Then it returns Refusal.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation, from _Flags of cmd/lark (C119, Q308)
func _Placed(asked *Asked) error {
	if asked.Verb == HELP && len(asked.Files) == 1 {
		asked.Topic = asked.Files[0]
	}

	if len(asked.Files) == 1 && asked.Verb != HELP {
		asked.File = asked.Files[0]
		asked.Path = asked.File
		asked.Noun = _Kind(asked.File)

		if asked.File == STDIN_ARG {
			asked.Stdin = true
			asked.Path = STDIN_NAME
			asked.Noun = SCRIPT

			if asked.As == AS_FLOW || asked.As == AS_BUNDLE {
				asked.Noun = asked.As
			}
		}
	}

	return Refusal(asked)
}

// _Kind is the kind of the file at path, from its extension (Q308): .bin is a
// bundle, .json a flow, and anything else a script.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q308)
func _Kind(path string) string {
	switch filepath.Ext(path) {
	case EXT_BUNDLE:
		return BUNDLE

	case EXT_FLOW:
		return FLOW

	default:
		return SCRIPT
	}
}
