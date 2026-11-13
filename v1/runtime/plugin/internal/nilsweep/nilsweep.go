// Package nilsweep holds the test support of the C87 nil sweep: the argument
// shapes a host or a plugin author could hand to a builtin, and a runner that
// calls a builtin with each of them and reports a panic or a hang.
//
// It serves tests only. No production package imports it.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
package nilsweep

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

// TIMEOUT is how long one call may take before the runner calls it a hang.
// BIG_BITS is the size of the huge number a shape holds, and WIDE_BITS the
// exponent of a width no memory can hold. FLOAT and OCTET are a float and a
// small width a shape holds.
//
// Revisions:
//   - 2026-11-04 19:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	TIMEOUT   = 10 * time.Second
	BIG_BITS  = 2000
	WIDE_BITS = 62
	FLOAT     = 1.5
	OCTET     = 8
)

// Shape is one set of arguments for a builtin.
//
// Revisions:
//   - 2026-11-04 19:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Shape struct {
	Name   string
	Args   starlark.Tuple
	Kwargs []starlark.Tuple
}

// Shapes returns the argument shapes: none, every kind of value a script
// holds, typed nils, numbers at the edges and well-formed keywords.
//
// No shape holds a nil Value in the arguments, and no keyword pair is
// malformed. go.starlark.net's UnpackArgs and UnpackPositionalArgs panic on
// both (they call Type on the nil, and index a pair they did not check), and
// about twenty plugins call them. Which seam guards that is a user decision
// (C87 report, "Found, not fixed"), so the sweep leaves those shapes out
// rather than fail on a panic that lives in the library.
//
// No shape asks for a large allocation: a width stays at or under 1 MiB, or
// is so large that the builtin must refuse it before it allocates.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func Shapes() []Shape {
	var shapes []Shape

	shapes = append(
		shapes,
		_Kinds()...,
	)
	shapes = append(
		shapes,
		_Numbers()...,
	)
	shapes = append(
		shapes,
		_Orders()...,
	)
	shapes = append(
		shapes,
		_Keywords()...,
	)

	return shapes
}

// _Kinds are the shapes of one argument of each kind of value.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Kinds() []Shape {
	return append(
		_Scalars(),
		_Containers()...,
	)
}

// _Scalars are the shapes of one argument that holds nothing.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Scalars() []Shape {
	huge := starlark.MakeBigInt(new(big.Int).Lsh(
		big.NewInt(1),
		BIG_BITS,
	))

	return []Shape{
		{
			Name: "no arguments",
		},
		{
			Name: "None",
			Args: starlark.Tuple{starlark.None},
		},
		{
			Name: "zero",
			Args: starlark.Tuple{starlark.MakeInt(0)},
		},
		{
			Name: "minus one",
			Args: starlark.Tuple{starlark.MakeInt(-1)},
		},
		{
			Name: "huge int",
			Args: starlark.Tuple{huge},
		},
		{
			Name: "bytes",
			Args: starlark.Tuple{starlark.Bytes("\xff\x00")},
		},
		{
			Name: "empty bytes",
			Args: starlark.Tuple{starlark.Bytes("")},
		},
		{
			Name: "text",
			Args: starlark.Tuple{starlark.String("zz")},
		},
		{
			Name: "empty text",
			Args: starlark.Tuple{starlark.String("")},
		},
		{
			Name: "float",
			Args: starlark.Tuple{starlark.Float(FLOAT)},
		},
	}
}

// _Containers are the shapes of one argument that holds other values,
// including a typed nil container.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Containers() []Shape {
	var (
		list   = starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		nested = starlark.NewList([]starlark.Value{nil})
		nilDct *starlark.Dict
		nilLst *starlark.List
	)

	return []Shape{
		{
			Name: "list",
			Args: starlark.Tuple{list},
		},
		{
			Name: "list with a nil",
			Args: starlark.Tuple{nested},
		},
		{
			Name: "dict",
			Args: starlark.Tuple{starlark.NewDict(0)},
		},
		{
			Name: "typed nil list and dict",
			Args: starlark.Tuple{nilLst, nilDct},
		},
		{
			Name: "empty tuple",
			Args: starlark.Tuple{starlark.Tuple{}},
		},
	}
}

// _Numbers are the shapes of a number with a width and a byte order.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Numbers() []Shape {
	var (
		huge = starlark.MakeBigInt(new(big.Int).Lsh(
			big.NewInt(1),
			BIG_BITS,
		))
		wide  = starlark.MakeInt64(1 << WIDE_BITS)
		one   = starlark.MakeInt(1)
		minus = starlark.MakeInt(-1)
		zero  = starlark.MakeInt(0)
		eight = starlark.MakeInt(OCTET)
		order = starlark.String("big")
	)

	return []Shape{
		{
			Name: "number and width",
			Args: starlark.Tuple{one, one},
		},
		{
			Name: "number and zero width",
			Args: starlark.Tuple{one, zero},
		},
		{
			Name: "number and negative width",
			Args: starlark.Tuple{one, minus},
		},
		{
			Name: "number and huge width",
			Args: starlark.Tuple{one, huge},
		},
		{
			Name: "number and 2**62 width",
			Args: starlark.Tuple{one, wide},
		},
		{
			Name: "huge number and width",
			Args: starlark.Tuple{huge, eight},
		},
		{
			Name: "negative number and width",
			Args: starlark.Tuple{minus, eight},
		},
		{
			Name: "number, width and order",
			Args: starlark.Tuple{one, one, order},
		},
		{
			Name: "too many arguments",
			Args: starlark.Tuple{one, one, one, one, one},
		},
		{
			Name: "text and width",
			Args: starlark.Tuple{starlark.String("zz"), one},
		},
	}
}

// _Orders are the shapes of bytes with a good and a bad byte order.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Orders() []Shape {
	var (
		one   = starlark.MakeInt(1)
		data  = starlark.Bytes("\xff")
		empty = starlark.String("")
	)

	return []Shape{
		{
			Name: "number, width and bad order",
			Args: starlark.Tuple{one, one, empty},
		},
		{
			Name: "bytes and order",
			Args: starlark.Tuple{data, starlark.String("little")},
		},
		{
			Name: "bytes and bad order",
			Args: starlark.Tuple{data, starlark.String("x")},
		},
	}
}

// _Keywords are the shapes of well-formed keyword arguments.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Keywords() []Shape {
	var (
		one  = starlark.MakeInt(1)
		name = starlark.String("zz")
	)

	return []Shape{
		{
			Name:   "unknown keyword",
			Kwargs: []starlark.Tuple{{name, one}},
		},
		{
			Name: "known keyword",
			Args: starlark.Tuple{one},
			Kwargs: []starlark.Tuple{
				{starlark.String("order"), starlark.String("big")},
			},
		},
	}
}

// Members returns the builtins of the plugin called name, which the test's
// import of its package registered, by member name.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func Members(
	t *testing.T,
	name string,
) map[string]*starlark.Builtin {
	t.Helper()

	for _, registered := range plugin.Registered() {
		if registered.Name() != name {
			continue
		}

		module, ok := registered.Values()[name].(*starlarkstruct.Module)
		if !ok {
			t.Fatalf(
				"plugin %s has no module",
				name,
			)
		}

		found := map[string]*starlark.Builtin{}

		for member, value := range module.Members {
			builtin, ok := value.(*starlark.Builtin)
			if ok {
				found[member] = builtin
			}
		}

		return found
	}

	t.Fatalf(
		"plugin %s is not registered",
		name,
	)

	return nil
}

// Receivers returns, for the plugin called name, the plugin with a nil pointer
// receiver and the plugin with a zero receiver: the two a host can make from
// the registered one by reflection or by a copy of its type.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-04 22:35: reads the type through the wrapper Register adds (Q134)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func Receivers(
	t *testing.T,
	name string,
) []common.Plugin {
	t.Helper()

	for _, registered := range plugin.Registered() {
		if registered.Name() != name {
			continue
		}

		kind := _Concrete(registered)

		return []common.Plugin{
			reflect.Zero(kind).Interface().(common.Plugin),
			reflect.New(kind.Elem()).Interface().(common.Plugin),
		}
	}

	t.Fatalf(
		"plugin %s is not registered",
		name,
	)

	return nil
}

// _Concrete is the type of the plugin a registered one holds. Register stores a
// plugin inside a wrapper that checks its builtins, and the wrapper keeps the
// plugin in its unexported field inner, which reflection reads without
// exporting it.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func _Concrete(registered common.Plugin) reflect.Type {
	held := reflect.ValueOf(registered)

	if held.Kind() == reflect.Pointer && held.Elem().Kind() == reflect.Struct {
		inner := held.Elem().FieldByName("inner")
		if inner.IsValid() && inner.Kind() == reflect.Interface {
			return inner.Elem().Type()
		}
	}

	return held.Type()
}

// Methods calls Name and Values on each receiver and fails t on a panic.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func Methods(
	t *testing.T,
	name string,
) {
	t.Helper()

	for index, receiver := range Receivers(
		t,
		name,
	) {
		func() {
			defer func() {
				problem := recover()
				if problem != nil {
					t.Errorf(
						"receiver %d: panicked: %v",
						index,
						problem,
					)
				}
			}()

			if receiver.Name() != name || len(receiver.Values()) == 0 {
				t.Errorf(
					"receiver %d: Name or Values changed",
					index,
				)
			}
		}()
	}
}

// Call calls fn once and returns what it recovered, or a hang.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func Call(
	fn *starlark.Builtin,
	thread *starlark.Thread,
	shape *Shape,
) any {
	finished := make(
		chan any,
		1,
	)

	go func() {
		defer func() {
			finished <- recover()
		}()

		_, _ = fn.CallInternal(
			thread,
			shape.Args,
			shape.Kwargs,
		)
	}()

	select {
	case problem := <-finished:
		return problem

	case <-time.After(TIMEOUT):
		return "hung"
	}
}

// Row is one call of a Go function, with the sentinel its error must wrap, or
// nil when any answer but a panic or a hang passes.
//
// Revisions:
//   - 2026-11-04 19:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Row struct {
	Name string
	Call func() error
	Want error
}

// Rows runs each row as a subtest and fails it on a panic, a hang, or an
// error that does not wrap Want.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func Rows(
	t *testing.T,
	rows []Row,
) {
	t.Helper()

	for _, row := range rows {
		t.Run(
			row.Name,
			func(t *testing.T) {
				type _Outcome struct {
					problem any
					err     error
				}

				finished := make(
					chan _Outcome,
					1,
				)

				go func() {
					var done _Outcome

					defer func() {
						done.problem = recover()
						finished <- done
					}()

					done.err = row.Call()
				}()

				select {
				case done := <-finished:
					if done.problem != nil {
						t.Fatalf(
							"panicked: %v",
							done.problem,
						)
					}

					if row.Want != nil && !errors.Is(
						done.err,
						row.Want,
					) {
						t.Fatalf(
							"error %v, want %v",
							done.err,
							row.Want,
						)
					}

				case <-time.After(TIMEOUT):
					t.Fatal("hung")
				}
			},
		)
	}
}

// Run calls fn with every shape, on a nil thread and on a thread, and fails t
// for each call that panics or hangs. The failures name the shapes.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
func Run(
	t *testing.T,
	fn *starlark.Builtin,
) {
	t.Helper()

	for _, thread := range []*starlark.Thread{nil, new(starlark.Thread)} {
		for _, shape := range Shapes() {
			problem := Call(
				fn,
				thread,
				&shape,
			)
			if problem != nil {
				t.Errorf(
					"%s: thread %v, %s: %s",
					shared.Called(fn),
					thread != nil,
					shape.Name,
					fmt.Sprint(problem),
				)
			}
		}
	}
}
