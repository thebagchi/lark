// Package graph is internal, not public: it derives the flow a script
// describes, checks one a host authored, and generates the script a flow
// describes.
//
// Revisions:
//   - 2026-09-21 01:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:06: moved under v1/runtime/internal; package doc says internal (C91w11)
package graph

import (
	"fmt"
	"strings"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// The words only the flow package reads. The vocabulary a script and a
// flow share (ENTRY, SPAWN, SUBJECT and the rest) is read from common.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 02:11: the aliases of the shared names dropped (C107, Q247)
const (
	// RANGE is the builtin a loop counts with.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	RANGE = "range"

	// DELAY is the keyword a repeat or a retry may name its pause with.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	DELAY = "delay"

	// TRUE and FALSE are how Starlark spells a boolean, and NONE its absence.
	//
	// Revisions:
	//   - 2026-09-20 21:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	TRUE  = "True"
	FALSE = "False"
	NONE  = "None"
)

// Reading is one derivation's own state.
//
// Separate from the Flow it writes because a struct that is both the walk and
// its result is one a caller can corrupt by reading it.
//
// Revisions:
//   - 2026-11-05 23:24: the text map holds *Lines in place of *_Lines (C60, Q188)
//   - 2026-11-10 00:25: renamed from _Reading, as a test in tests/ needs it (C120a, Q314)
type Reading struct {
	walk      *script.Walk
	entry     string
	defs      map[string]*syntax.DefStmt
	text      map[string]*Lines
	owner     map[string]string
	builtin   map[string]string
	order     []string
	init      []*workflowpb.Init
	constants map[string]*workflowpb.Constant
	args      map[string]*workflowpb.Arg
	read      map[string]*_File
	files     []*_File
	loaded    map[*syntax.LoadStmt]*syntax.File
}

// _File is one parsed and resolved file, read but not yet declared: the flow
// declares what a file holds only after the shake has said which statements a
// run reaches, and that needs every file.
//
// Revisions:
//   - 2026-11-05 23:24: the text field is a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-11 00:33: drops source, which only the texts handed to Reach read (C122, B263)
type _File struct {
	name string
	tree *syntax.File
	text *Lines
}

// Of reads a script and returns the flow it describes.
//
// A function the walk cannot model is not a giving-up: it travels as body text,
// which the flow carries. So a flow is the whole script, and what cannot be
// carried at all is refused rather than left out.
//
// Each function, and main, is either statements or body text: one line the
// walk cannot model makes the whole of it text. Parsing goes through
// common.OPTIONS, so this uses the dialect decision rather than owning one.
//
// Returns spelling.ErrMain for a module or a nil source, ErrCarry for what the
// flow cannot carry (a constant, a signature, a load that renames, a top-level
// statement of a loaded module), ErrDuplicate for two modules that declare one
// name, ErrShadow for a declaration that hides a builtin another file reads,
// and spelling.ErrCycle for modules that load each other.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 08:09: names its source for what it is
//   - 2026-09-22 22:41: reads the arguments a script declares
//   - 2026-10-01 23:57: takes a Source and writes a Flow, a function's
//     statements or its body text rather than a list of threads
//   - 2026-10-03 00:10: returns the flow alone, refusing a cycle of loads
//     rather than reporting it beside the flow, and takes script's Source
//   - 2026-11-02 17:53: refuses a top-level statement it would drop, and a
//     declaration that hides a builtin another file reads (C43, B042, B043)
//   - 2026-11-04 19:26: carries the entry's own top-level statements in the
//     flow, and refuses them only in a loaded module (C44c, B054)
//   - 2026-11-05 18:00: declares only what a run reaches, by the rule a compile
//     applies (C44b, Q164)
//   - 2026-11-05 23:24: the text map is made of *Lines in place of *_Lines (C60, Q188)
//   - 2026-11-06 00:16: bounds the entry name in a refusal (C63, B128, Q209)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Reading, Whole, Shadows, Flow, were _Reading, _Whole, _Shadows,
//     _Flow (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrMain and ErrCarry, formerly ErrNoMain and ErrNotCarried (C121,
//     Q348)
//   - 2026-11-12 23:40: reads the source through script.Normalised (C130, Q405)
func Of(source *common.Source) (*workflowpb.Flow, error) {
	source = script.Normalised(source)
	if source == nil {
		return nil, spelling.ErrMain
	}

	tree, err := common.OPTIONS.Parse(
		unpack.ShortText(source.Entry),
		source.Text,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(source.Entry),
			err,
		)
	}

	reading := &Reading{
		walk: script.NewWalk(
			script.LoaderFor(source),
			source.Entry,
		),
		defs:      make(map[string]*syntax.DefStmt),
		text:      make(map[string]*Lines),
		owner:     make(map[string]string),
		builtin:   make(map[string]string),
		entry:     source.Entry,
		constants: make(map[string]*workflowpb.Constant),
		args:      make(map[string]*workflowpb.Arg),
		read:      make(map[string]*_File),
		loaded:    make(map[*syntax.LoadStmt]*syntax.File),
	}

	err = reading.Whole(
		tree,
		string(source.Text),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(source.Entry),
			err,
		)
	}

	err = reading.Shadows()
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(source.Entry),
			err,
		)
	}

	if reading.defs[common.ENTRY] == nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(source.Entry),
			spelling.ErrMain,
		)
	}

	return reading.Flow(), nil
}

// Constants is every module-level name a body may read: the arguments a run
// supplies, and the values that are fixed.
//
// A constant is a literal, or a call of a function this flow declares. Some
// constants are computed, and a Call naming a declared function stays
// inspectable and editable where a string of Starlark would not - arbitrary
// code before the entry point is still refused, and a named call of a declared
// function is not that.
//
// The two share one namespace, and one owner map enforces it: both bind a
// module-level name, so a name in both is the same collision as a name in two
// modules.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-22 22:41: sorts an argument declaration into args, leaving the
//     rest to be carried as constants
//   - 2026-11-02 17:53: refuses a statement that is no def, load or assignment,
//     which it left out without a message (C43, B042)
//   - 2026-11-04 19:26: takes the file's lines, so the entry's statements,
//     an augmented assignment among them, are carried (C44c, B054)
//   - 2026-11-05 18:00: skips the statements the shake left out (C44b, Q164)
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 22:02: a nil tree has no constants (C100c, B179)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: renamed from _Constants, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
//   - 2026-11-13 00:47: carries a bytes constant as an init statement (C130, B290, Q406)
func (r *Reading) Constants(
	tree *syntax.File,
	module string,
	text *Lines,
	keep []bool,
) error {
	if tree == nil {
		return nil
	}

	for idx, stmt := range tree.Stmts {
		if keep != nil && !keep[idx] {
			continue
		}

		assign, ok := stmt.(*syntax.AssignStmt)
		if !ok || r._Inited(
			assign,
			module,
		) {
			err := r.Placed(
				stmt,
				module,
				text,
			)
			if err != nil {
				return err
			}

			continue
		}

		name, ok := assign.LHS.(*syntax.Ident)
		if !ok || assign.Op != syntax.EQ {
			return fmt.Errorf(
				"%s: constant %w",
				unpack.QuotePrefix(module),
				ErrCarry,
			)
		}

		owner, known := r.owner[name.Name]
		if known && owner != module {
			return fmt.Errorf(
				"%s in %s and %s: %w",
				unpack.ShortText(name.Name),
				unpack.QuotePrefix(owner),
				unpack.QuotePrefix(module),
				ErrDuplicate,
			)
		}

		err := r._Bound(
			module,
			name.Name,
			assign.RHS,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// Runs reports whether an assignment of the entry is a statement that runs
// rather than a constant: an augmented assignment, or one to a target that is
// no name, such as an element.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Runs, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Runs(
	assign *syntax.AssignStmt,
	module string,
) bool {
	if r == nil {
		return false
	}

	if module != r.entry {
		return false
	}

	_, named := assign.LHS.(*syntax.Ident)

	return !named || assign.Op != syntax.EQ
}

// Placed puts a top-level statement that is no def, no load and no constant
// where the flow carries it, or refuses it.
//
// A call such as print("init"), an assert, an if, a for, an augmented
// assignment runs when the file is read. The entry script's statements are
// carried in the flow's init, one entry for each, which a generator writes
// after the arguments and before main. A loaded module's are refused: a flow
// has one init, and a module's statement has no place in it. Leaving a
// statement out generated a script that ran without it and said nothing.
//
// The line is in the error, the text of the statement is not: it comes from
// the script.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, B042)
//   - 2026-11-04 19:26: carries the entry's statement in init instead of
//     exempting it (C44c, B054)
//   - 2026-11-05 23:26: takes a *Lines, and cuts with its At and End in place of _At and
//     _LineEnd (C60, Q188)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: renamed from _Placed, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 11:58: refuses a statement whose end the text lacks (C118, B260, Q65)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
//   - 2026-11-11 22:57: refuses a statement that spawns or times out; the cut of the init
//     entry moved to _Written (C127, B274, Q383, Q388, Q389)
func (r *Reading) Placed(
	stmt syntax.Stmt,
	module string,
	text *Lines,
) error {
	if r == nil {
		return _NoReading()
	}

	switch stmt.(type) {
	case *syntax.DefStmt, *syntax.LoadStmt:
		return nil
	}

	start, _ := stmt.Span()

	if module != r.entry {
		return fmt.Errorf(
			"%s:%d: top-level statement %w",
			unpack.QuotePrefix(module),
			start.Line,
			ErrCarry,
		)
	}

	err := r._Unstarted(
		stmt,
		module,
	)
	if err != nil {
		return err
	}

	held, err := r._Written(
		stmt,
		module,
		text,
	)
	if err != nil {
		return err
	}

	r.init = append(
		r.init,
		held,
	)

	return nil
}

// Flow is the flow this reading saw.
//
// main is not one of the functions: it is the spine, written from Flow.main or
// Flow.text, and the list does not begin with a call of it.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 23:53: the spine names the entry point in its first step, as
//     every thread names what it runs
//   - 2026-09-22 22:41: carries the arguments the script declared
//   - 2026-10-01 23:57: writes a Flow: the functions, then main as
//     statements or text
//   - 2026-10-03 00:10: returns the flow alone, nothing being left out of it
//   - 2026-11-04 19:26: carries the entry's top-level statements as init
//     (C44c, B054)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Flow, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Flow() *workflowpb.Flow {
	if r == nil {
		return nil
	}

	flow := new(workflowpb.Flow)

	for _, name := range r.order {
		if name == common.ENTRY {
			continue
		}

		flow.Functions = append(
			flow.Functions,
			r.Function(r.defs[name]),
		)
	}

	if len(r.constants) > 0 {
		flow.Constants = r.constants
	}

	if len(r.args) > 0 {
		flow.Args = r.args
	}

	r.Spine(flow)

	flow.Init = r.init

	return flow
}

// Function is one top-level def as the flow carries it: its name, its
// parameters, and either its statements or its body text.
//
// Statements when every line models, text when one does not. Never both: a
// function carried as statements and as text has said one thing twice in two
// languages.
//
// Every def reaching here has a plain signature: Declare refused the others.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: no longer records a signature it cannot carry, since
//     one never gets this far
//   - 2026-10-01 23:57: carries the def's own statements, or its text
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Function, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Function(def *syntax.DefStmt) *workflowpb.Function {
	if r == nil {
		return nil
	}

	var params, _ = Params(def)

	var (
		fn = &workflowpb.Function{
			Name:   def.Name.Name,
			Params: params,
		}
		stmts, ok = r._Modelled(
			def,
			params,
		)
	)
	if ok {
		fn.Code = &workflowpb.Function_Statements{
			Statements: &workflowpb.Statements{
				Statement: stmts,
			},
		}

		return fn
	}

	fn.Code = &workflowpb.Function_Body{
		Body: _Body(
			r.text[def.Name.Name],
			def,
		),
	}

	return fn
}

// Spine is main as the flow carries it: statements when every line models,
// the inside of the def as text when one does not.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 12:59: a nil reading does nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Spine, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Spine(flow *workflowpb.Flow) {
	if r == nil {
		return
	}

	var def = r.defs[common.ENTRY]

	stmts, ok := r._Modelled(
		def,
		nil,
	)
	if ok {
		flow.Spine = &workflowpb.Flow_Main{
			Main: &workflowpb.Statements{
				Statement: stmts,
			},
		}

		return
	}

	flow.Spine = &workflowpb.Flow_Text{
		Text: _Body(
			r.text[common.ENTRY],
			def,
		),
	}
}

// Listed reports whether name is a function the flow lists, which is what a
// call must name to be a statement.
//
// main is declared and not listed: it is the spine, and nothing calls it.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:02: a nil reading lists nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Listed, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Listed(name string) bool {
	if r == nil {
		return false
	}

	return name != common.ENTRY && r.defs[name] != nil
}

// Builtin reports whether name means the builtin of that name: the script
// declares no function, constant or argument over it.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
//   - 2026-11-08 13:02: a nil reading means no builtin (C108, B187)
//   - 2026-11-10 00:25: renamed from _Builtin, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Builtin(name string) bool {
	if r == nil {
		return false
	}

	var (
		_, constant = r.constants[name]
		_, arg      = r.args[name]
	)

	return r.defs[name] == nil && !constant && !arg
}

// _Holding reports whether an expression is a call of a function the flow lists,
// or of a builtin, whose arguments hold a dict with a key that is no string and
// that would be a constant if each such key were a string. A call of the builtin
// arg is a declaration that _Defaults answers for; a malformed one is not a call
// _Held takes, so it is refused as it is for a string.
//
// Revisions:
//   - 2026-11-13 01:21: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: asks for a dict key that is no string, where it asked for a bytes literal;
//     no longer sets a call of arg aside, which changed nothing (C132, Q406, Q407, Q423)
func (r *Reading) _Holding(expr syntax.Expr) bool {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return false
	}

	stringed, changed := _Stringed(call)
	if !changed {
		return false
	}

	_, err := r._Held(stringed)

	return err == nil
}

// _Inited reports whether an assignment of the entry is carried as the text of
// an init statement, not as a constant: one that runs, or one that states a value
// that holds a dict with a key that is no string.
//
// Revisions:
//   - 2026-11-13 00:47: initial creation, out of Constants (C130, B290, Q406)
//   - 2026-11-13 10:15: calls _Textual, was _Bytes (C132, Q406, Q407)
func (r *Reading) _Inited(
	assign *syntax.AssignStmt,
	module string,
) bool {
	return r.Runs(
		assign,
		module,
	) || r._Textual(
		assign,
		module,
	)
}

// _Textual reports whether an assignment of the entry states a value that holds a
// dict with a key that is no string, so that it is carried as the text of an init
// statement. It is asked after Runs, so the assignment is of one name with an
// equals sign. A call of arg with such a dict as its default is text whether arg is
// the builtin or a function of the script: the text is what the script wrote.
//
// A map of the flow has string keys only (Q423), so a constant that holds a dict
// with another key has no place among the constants of a flow. The statement as
// the script wrote it does (Q406).
//
// Revisions:
//   - 2026-11-13 00:47: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: renamed from _Bytes, and asks for a dict key that is no string, where it
//     asked for a bytes literal, which a flow value holds now; no longer asks whether the
//     script defines arg, which changed nothing (C132, Q406, Q407, Q423)
func (r *Reading) _Textual(
	assign *syntax.AssignStmt,
	module string,
) bool {
	if r == nil || module != r.entry {
		return false
	}

	return _Keyed(assign.RHS) ||
		_Defaults(assign.RHS) ||
		r._Holding(assign.RHS)
}

// _Unstarted refuses a top-level statement that calls a builtin script.Starts
// names - spawn, timeout - which the builtin refuses while the top level runs
// (Q383, Q389). A script that declares its own function of that name is left
// alone for it.
//
// Revisions:
//   - 2026-11-11 22:58: initial creation, as _Unspawned; takes every builtin script.Starts
//     names, in place of spawn alone (C127, B274, Q383, Q388, Q389)
func (r *Reading) _Unstarted(
	stmt syntax.Stmt,
	module string,
) error {
	name := _Starting(
		stmt,
		r.Builtin,
	)
	if name == "" {
		return nil
	}

	start, _ := stmt.Span()

	return _Barring(
		fmt.Sprintf(
			"%s:%d: top-level statement %s",
			unpack.QuotePrefix(module),
			start.Line,
			ErrCarry,
		),
		ErrCarry,
		name,
	)
}

// _Written is a top-level statement of the entry as an init entry: a call
// when it is one, and otherwise its text, cut from the file.
//
// The text ends where the statement ends when another statement follows it on
// its line, and at the end of the line when none does, so an end-of-line comment
// stays with the statement it follows, and a statement after a semicolon is
// written once (B288).
//
// Revisions:
//   - 2026-11-11 22:58: initial creation, out of Placed (C127, B274, Q383)
//   - 2026-11-12 22:11: ends the text of a statement that another follows on its line where
//     it ends (C130, B288)
func (r *Reading) _Written(
	stmt syntax.Stmt,
	module string,
	text *Lines,
) (*workflowpb.Init, error) {
	held, ok := r._Initial(
		stmt,
		text,
	)
	if !ok {
		var (
			start, _ = stmt.Span()
			from     = text.At(
				start.Line,
				start.Col,
			)
			to = r._Ending(
				stmt,
				text,
			)
		)
		if from == MISSING || to == MISSING || to < from {
			return nil, fmt.Errorf(
				"%s:%d: top-level statement %w",
				unpack.QuotePrefix(module),
				start.Line,
				ErrCarry,
			)
		}

		held = &workflowpb.Init{
			Form: &workflowpb.Init_Text{
				Text: text.src[from:to],
			},
		}
	}

	return held, nil
}

// _Ending is the byte offset where the text of a top-level statement ends: the
// end of the statement when a semicolon after it is followed by another statement
// on its last line, and the end of that line otherwise.
//
// The semicolon is found with Semicolon, which skips the closing bracket the
// parser ends an index, a slice and a parenthesised tuple at; the text then ends
// before the semicolon, without the blanks before it.
//
// Revisions:
//   - 2026-11-12 22:49: initial creation, out of _Written (C130, B288)
func (r *Reading) _Ending(
	stmt syntax.Stmt,
	text *Lines,
) int {
	var (
		_, end = stmt.Span()
		at     = text.At(
			end.Line,
			end.Col,
		)
		line = text.End(end.Line)
	)

	if at == MISSING || at > line {
		return line
	}

	after, found := Semicolon(
		text.src[:line],
		at,
	)
	if !found || !Following(
		text.src[:line],
		after,
	) {
		return line
	}

	return len(strings.TrimRight(
		text.src[:after-1],
		" \t",
	))
}

// _Initial is a top-level statement as a call, and whether it is one.
//
// A call of a function the flow declares, or of a builtin, with keywords and
// expressions as arguments if it likes. Any other statement is the caller's
// to carry as text.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-04 19:26: reads any callee, keywords and expressions (C44c,
//     B054)
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-10 01:00: now uses Reading, were _Reading (C120a, Q314)
func (r *Reading) _Initial(
	stmt syntax.Stmt,
	text *Lines,
) (*workflowpb.Init, bool) {
	expr, ok := stmt.(*syntax.ExprStmt)
	if !ok {
		return nil, false
	}

	call, ok := expr.X.(*syntax.CallExpr)
	if !ok {
		return nil, false
	}

	held, ok := r._Extended(
		call,
		r._Initing(text),
	)
	if !ok {
		return nil, false
	}

	return &workflowpb.Init{
		Form: &workflowpb.Init_Call{
			Call: held,
		},
	}, true
}

// _Bound records one assignment as an argument or as a constant.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-10 01:00: now uses Reading, were _Reading (C120a, Q314)
func (r *Reading) _Bound(
	module string,
	name string,
	expr syntax.Expr,
) error {
	declared, err := r._Declared(expr)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			unpack.ShortText(name),
			err,
		)
	}

	if declared != nil {
		r.args[name] = declared
		r.owner[name] = module

		return nil
	}

	held, err := r._Held(expr)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			unpack.ShortText(name),
			err,
		)
	}

	r.constants[name] = held
	r.owner[name] = module

	return nil
}

// _Held is the constant an expression states.
//
// A call may pass literals, and names of the functions and constants declared
// before it: a constant is computed at the top of a file, where an argument or
// a later constant is not bound yet.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-01 23:57: a call may pass the name of a constant declared
//     before it, or of a function
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Reading, Call, were _Reading, _Call (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func (r *Reading) _Held(expr syntax.Expr) (*workflowpb.Constant, error) {
	value, ok := _Arg(expr)
	if ok {
		return &workflowpb.Constant{
			Kind: &workflowpb.Constant_Value{
				Value: value,
			},
		}, nil
	}

	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, fmt.Errorf(
			"constant %w",
			ErrCarry,
		)
	}

	held, ok := r.Call(
		call,
		r._Top(),
	)
	if !ok {
		return nil, fmt.Errorf(
			"constant %w",
			ErrCarry,
		)
	}

	return &workflowpb.Constant{
		Kind: &workflowpb.Constant_Call{
			Call: held,
		},
	}, nil
}

// _Prior reports whether a constant's call may pass name: a constant declared
// before it, or a function.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
//   - 2026-11-08 12:46: a nil reading declares nothing (C108, B187)
//   - 2026-11-10 01:00: now uses Reading, Listed, were _Reading, _Listed (C120a, Q314)
func (r *Reading) _Prior(name string) bool {
	if r == nil {
		return false
	}

	_, constant := r.constants[name]

	return constant || r.Listed(name)
}

// _Callee is the plain name a call calls, or empty.
//
// A call through anything but a bare identifier - a method, an element of a
// list, a value another call returned - is one a flow cannot point at. That is
// also what keeps " ".join(parts) out: it is a DotExpr, so it never reaches the
// join a workflow means.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation, as _Bare
//   - 2026-10-01 23:57: named for what it returns, since the node builders
//     took _Bare
//   - 2026-11-08 10:35: a nil call names nothing (C108, B190)
func _Callee(call *syntax.CallExpr) string {
	if call == nil {
		return ""
	}

	name, ok := call.Fn.(*syntax.Ident)
	if !ok {
		return ""
	}

	return name.Name
}

// _NoReading is the error a method of a nil reading returns: there is nothing
// to read, so the flow has no form.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B187)
func _NoReading() error {
	return fmt.Errorf(
		"reading: %w",
		ErrForm,
	)
}
