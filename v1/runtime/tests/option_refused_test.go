package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Supplier is a host plugin that supplies one name, and nothing a script calls.
//
// Revisions:
//   - 2026-11-08 10:30: initial creation (C75, Q235)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Supplier struct {
	name string
}

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-08 10:30: initial creation (C75, Q235)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (n *_Supplier) Name() string {
	return n.name
}

// Values supplies one name, bound to None.
//
// Revisions:
//   - 2026-11-08 10:30: initial creation (C75, Q235)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (n *_Supplier) Values() starlark.StringDict {
	return starlark.StringDict{
		n.name: starlark.None,
	}
}

// TestOptions_RefuseRepeats proves Q235: a second WithPlugins, and a second
// WithLog, are refused with an error that names the option, and a single use
// of each still works.
//
// Revisions:
//   - 2026-11-08 10:30: initial creation (C75, Q235)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate; a second WithLog replaces WithLog
//     beside WithLogger, which is dropped (C121, Q339, Q340)
func TestOptions_RefuseRepeats(t *testing.T) {
	var (
		first = &_Supplier{
			name: "host_a",
		}
		second = &_Supplier{
			name: "host_b",
		}
	)

	t.Run(
		"compile",
		func(t *testing.T) {
			tests := []struct {
				name string
				opts []common.CompileOption
				want string
			}{
				{
					name: "one WithPlugins",
					opts: []common.CompileOption{
						runtime.WithPlugins(first),
					},
				},
				{
					name: "no option",
				},
				{
					name: "two WithPlugins",
					opts: []common.CompileOption{
						runtime.WithPlugins(first),
						runtime.WithPlugins(second),
					},
					want: "WithPlugins",
				},
				{
					name: "two empty WithPlugins are no calls",
					opts: []common.CompileOption{
						runtime.WithPlugins(),
						runtime.WithPlugins(),
					},
				},
				{
					name: "a nil option between",
					opts: []common.CompileOption{
						runtime.WithPlugins(first),
						nil,
						runtime.WithPlugins(second),
					},
					want: "WithPlugins",
				},
			}

			for _, tc := range tests {
				t.Run(
					tc.name,
					func(t *testing.T) {
						_, err := runtime.Compile(
							_NilGuardSource(),
							tc.opts...,
						)
						_CheckRefusal(
							t,
							err,
							tc.want,
						)

						_, err = runtime.Merge(
							_NilGuardSource(),
							tc.opts...,
						)
						_CheckRefusal(
							t,
							err,
							tc.want,
						)
					},
				)
			}
		},
	)

	t.Run(
		"run",
		func(t *testing.T) {
			tests := []struct {
				name string
				opts func(w *bytes.Buffer) []common.RunOption
				want string
			}{
				{
					name: "WithLog alone",
					opts: func(w *bytes.Buffer) []common.RunOption {
						return []common.RunOption{runtime.WithLog(w)}
					},
				},
				{
					name: "WithLog twice",
					opts: func(w *bytes.Buffer) []common.RunOption {
						return []common.RunOption{
							runtime.WithLog(w),
							runtime.WithLog(new(bytes.Buffer)),
						}
					},
					want: "WithLog",
				},
			}

			for _, tc := range tests {
				t.Run(
					tc.name,
					func(t *testing.T) {
						var out bytes.Buffer

						_, err := runtime.Run(
							context.Background(),
							_NilGuardSource(),
							tc.opts(&out)...,
						)
						_CheckRefusal(
							t,
							err,
							tc.want,
						)

						if tc.want != "" &&
							!strings.Contains(
								err.Error(),
								"WithLog is given twice",
							) {
							t.Errorf(
								"error %q lacks it",
								err,
							)
						}

						if tc.want != "" && out.Len() != 0 {
							t.Errorf(
								"a refused run wrote %q",
								out.String(),
							)
						}
					},
				)
			}
		},
	)
}

// TestOptions_RefuseRepeatedValues proves Q255 and Q256: each option that
// takes a value is refused when given twice, with the exact plain text, and a
// refusal joins the next one rather than hiding it. WithTracking, a flag,
// stays idempotent.
//
// Revisions:
//   - 2026-11-08 11:30: initial creation (C75, Q255, Q256)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.WithTracking, formerly WithLiveGraph, and the log cases use
//     WithLog alone, WithLogger being dropped (C121, Q340, Q341)
func TestOptions_RefuseRepeatedValues(t *testing.T) {
	var (
		change = func(*workflowpb.Change) {
			_ = 0
		}
		bad = map[string]any{"x": make(chan int)}
	)

	tests := []struct {
		name string
		opts []common.RunOption
		want []string
	}{
		{
			name: "WithArgs twice",
			opts: []common.RunOption{
				runtime.WithArgs(map[string]any{}),
				runtime.WithArgs(map[string]any{}),
			},
			want: []string{"WithArgs is given twice (give it once)"},
		},
		{
			name: "WithArgs refused then good: still a repeat",
			opts: []common.RunOption{
				runtime.WithArgs(bad),
				runtime.WithArgs(map[string]any{}),
			},
			want: []string{
				"an argument holds a value JSON cannot",
				"WithArgs is given twice (give it once)",
			},
		},
		{
			name: "WithArgs refused twice: still a repeat",
			opts: []common.RunOption{
				runtime.WithArgs(bad),
				runtime.WithArgs(bad),
			},
			want: []string{
				"an argument holds a value JSON cannot",
				"WithArgs is given twice (give it once)",
			},
		},
		{
			name: "WithChanges twice",
			opts: []common.RunOption{
				runtime.WithChanges(change),
				runtime.WithChanges(change),
			},
			want: []string{"WithChanges is given twice (give it once)"},
		},
		{
			name: "WithMemoryWatch twice",
			opts: []common.RunOption{
				runtime.WithMemoryWatch(_Abandon),
				runtime.WithMemoryWatch(_Abandon),
			},
			want: []string{"WithMemoryWatch is given twice (give it once)"},
		},
		{
			name: "WithLog twice",
			opts: []common.RunOption{
				runtime.WithLog(new(bytes.Buffer)),
				runtime.WithLog(new(bytes.Buffer)),
			},
			want: []string{"WithLog is given twice (give it once)"},
		},
		{
			name: "WithTracking twice is fine",
			opts: []common.RunOption{
				runtime.WithTracking(),
				runtime.WithTracking(),
			},
		},
		{
			name: "bad args then a repeated log: both refusals kept",
			opts: []common.RunOption{
				runtime.WithArgs(bad),
				runtime.WithLog(new(bytes.Buffer)),
				runtime.WithLog(new(bytes.Buffer)),
			},
			want: []string{
				"an argument holds a value JSON cannot",
				"WithLog is given twice (give it once)",
			},
		},
		{
			name: "a repeated log then bad args: both refusals kept",
			opts: []common.RunOption{
				runtime.WithLog(new(bytes.Buffer)),
				runtime.WithLog(new(bytes.Buffer)),
				runtime.WithArgs(bad),
			},
			want: []string{
				"an argument holds a value JSON cannot",
				"WithLog is given twice (give it once)",
			},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Run(
					context.Background(),
					_NilGuardSource(),
					tc.opts...,
				)
				if len(tc.want) == 0 {
					if err != nil {
						t.Fatalf(
							"want no error, got %v",
							err,
						)
					}

					return
				}

				if err == nil {
					t.Fatal("want a refusal, got none")
				}

				for _, text := range tc.want {
					if !strings.Contains(
						err.Error(),
						text,
					) {
						t.Errorf(
							"error %q misses %q",
							err,
							text,
						)
					}
				}
			},
		)
	}
}

// TestOptions_PluginsRefusalText proves Q255: the text of a second WithPlugins.
//
// Revisions:
//   - 2026-11-08 11:30: initial creation (C75, Q255)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestOptions_PluginsRefusalText(t *testing.T) {
	_, err := runtime.Compile(
		_NilGuardSource(),
		runtime.WithPlugins(&_Supplier{
			name: "host_a",
		}),
		runtime.WithPlugins(&_Supplier{
			name: "host_b",
		}),
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"WithPlugins is given twice (give all plugins in one call)",
		) {
		t.Fatalf(
			"unexpected error %v",
			err,
		)
	}
}

// _CheckRefusal fails the test unless err is nil when want is empty, or is
// an error whose text holds want when it is not.
//
// Revisions:
//   - 2026-11-08 10:30: initial creation (C75, Q235)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _CheckRefusal(
	t *testing.T,
	err error,
	want string,
) {
	t.Helper()

	switch {
	case want == "" && err != nil:
		t.Fatalf(
			"want no error, got %v",
			err,
		)
	case want != "" && err == nil:
		t.Fatalf(
			"want an error naming %s, got none",
			want,
		)
	case want != "" && !strings.Contains(
		err.Error(),
		want,
	):
		t.Fatalf(
			"error %q does not name %s",
			err,
			want,
		)
	}
}

// _Abandon is an abandon function that does nothing.
//
// Revisions:
//   - 2026-11-08 12:30: initial creation (C75, Q261)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Abandon(error) {
	_ = 0
}

// TestOptions_NilIsNotGiven proves Q261: a nil value is no call. A nil call,
// then a real one, is accepted and the real value wins; two real calls are
// still refused.
//
// Revisions:
//   - 2026-11-08 12:30: initial creation (C75, Q261)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:26: drops the WithLogger case, WithLogger being dropped (C121, Q340)
func TestOptions_NilIsNotGiven(t *testing.T) {
	const SCRIPT = "greeting = arg(\"greeting\", \"none\")\n" +
		"def main():\n" +
		"    print(\"hi\")\n" +
		"    return greeting\n"

	source := func() *common.Source {
		return &common.Source{
			Entry: "n.star",
			Text:  []byte(SCRIPT),
		}
	}

	t.Run(
		"WithPlugins",
		func(t *testing.T) {
			for name, opts := range map[string][]common.CompileOption{
				"nil plugin then real": {
					runtime.WithPlugins(nil),
					runtime.WithPlugins(&_Supplier{
						name: "host_a",
					}),
				},
				"typed nil then real": {
					runtime.WithPlugins((*_Supplier)(nil)),
					runtime.WithPlugins(&_Supplier{
						name: "host_a",
					}),
				},
				"none then real": {
					runtime.WithPlugins(),
					runtime.WithPlugins(&_Supplier{
						name: "host_a",
					}),
				},
			} {
				_, err := runtime.Compile(
					&common.Source{
						Entry: "p.star",
						Text:  []byte("def main():\n    return host_a\n"),
					},
					opts...,
				)
				if err != nil {
					t.Errorf(
						"%s: %v",
						name,
						err,
					)
				}
			}
		},
	)

	t.Run(
		"WithArgs",
		func(t *testing.T) {
			value, err := runtime.Run(
				context.Background(),
				source(),
				runtime.WithArgs(nil),
				runtime.WithArgs(map[string]any{"greeting": "yo"}),
			)
			if err != nil || value.String() != `"yo"` {
				t.Fatalf(
					"got %v, %v",
					value,
					err,
				)
			}
		},
	)

	t.Run(
		"WithLog",
		func(t *testing.T) {
			var out bytes.Buffer

			_, err := runtime.Run(
				context.Background(),
				source(),
				runtime.WithLog(nil),
				runtime.WithLog(&out),
			)
			if err != nil || !strings.Contains(
				out.String(),
				"hi",
			) {
				t.Fatalf(
					"got %q, %v",
					out.String(),
					err,
				)
			}
		},
	)

	t.Run(
		"WithChanges",
		func(t *testing.T) {
			var calls int

			_, err := runtime.Run(
				context.Background(),
				source(),
				runtime.WithChanges(nil),
				runtime.WithChanges(func(*workflowpb.Change) {
					calls++
				}),
			)
			if err != nil || calls == 0 {
				t.Fatalf(
					"calls %d, %v",
					calls,
					err,
				)
			}
		},
	)

	t.Run(
		"WithMemoryWatch",
		func(t *testing.T) {
			_, err := runtime.Run(
				context.Background(),
				source(),
				runtime.WithMemoryWatch(nil),
				runtime.WithMemoryWatch(_Abandon),
			)
			if err != nil {
				t.Fatal(err)
			}

			_, err = runtime.Run(
				context.Background(),
				source(),
				runtime.WithMemoryWatch(nil),
				runtime.WithMemoryWatch(_Abandon),
				runtime.WithMemoryWatch(_Abandon),
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					"WithMemoryWatch is given twice",
				) {
				t.Fatalf(
					"want the refusal, got %v",
					err,
				)
			}
		},
	)
}

// TestOptions_OneOptionManyRuns proves an option built once is safe to hand to
// runs that go at the same time: applying it only reads what it captured. Run
// under -race, a write to captured state is a data race (C75 test round 2).
//
// Revisions:
//   - 2026-11-08 13:00: initial creation (C75)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval and runtime.WithTracking, formerly Evaluate and
//     WithLiveGraph, and drops the WithLogger case, WithLogger being dropped (C121, Q338, Q340,
//     Q341)
func TestOptions_OneOptionManyRuns(t *testing.T) {
	const RUNS = 8

	var calls atomic.Int64

	tests := []struct {
		name string
		opt  common.RunOption
	}{
		{
			name: "WithArgs",
			opt: runtime.WithArgs(map[string]any{
				"greeting": "yo",
			}),
		},
		{
			name: "WithArgs refused",
			opt: runtime.WithArgs(map[string]any{
				"greeting": make(chan int),
			}),
		},
		{
			name: "WithLog",
			opt:  runtime.WithLog(io.Discard),
		},
		{
			name: "WithChanges",
			opt: runtime.WithChanges(func(*workflowpb.Change) {
				calls.Add(1)
			}),
		},
		{
			name: "WithMemoryWatch",
			opt:  runtime.WithMemoryWatch(_Abandon),
		},
		{
			name: "WithMemoryWatch nil",
			opt:  runtime.WithMemoryWatch(nil),
		},
		{
			name: "WithTracking",
			opt:  runtime.WithTracking(),
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				built, err := runtime.Compile(&common.Source{
					Entry: "m.star",
					Text: []byte(
						"greeting = arg(\"greeting\", \"none\")\n" +
							"def main():\n" +
							"    return greeting\n",
					),
				})
				if err != nil {
					t.Fatal(err)
				}

				var group sync.WaitGroup

				for range RUNS {
					group.Go(func() {
						_, _ = runtime.Eval(
							context.Background(),
							built,
							tc.opt,
						)
					})
				}

				group.Wait()
			},
		)
	}

	t.Run(
		"WithPlugins",
		func(t *testing.T) {
			opt := runtime.WithPlugins(&_Supplier{
				name: "host_a",
			})

			var group sync.WaitGroup

			for range RUNS {
				group.Go(func() {
					_, _ = runtime.Compile(
						_NilGuardSource(),
						opt,
					)
				})
			}

			group.Wait()
		},
	)
}
