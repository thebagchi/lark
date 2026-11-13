package tests

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
)

// _Fields walks the fields that are set in m, and in the messages inside them, in the
// order of the schema and of the sorted keys of a map. It counts each field in index.
// When the count reaches target, it calls apply with the message and the field and
// stops; it returns whether it did.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func _Fields(
	m protoreflect.Message,
	index *int,
	target int,
	apply func(
		protoreflect.Message,
		protoreflect.FieldDescriptor,
	),
) bool {
	fields := m.Descriptor().Fields()

	for position := range fields.Len() {
		field := fields.Get(position)
		if !m.Has(field) {
			continue
		}

		if *index == target {
			apply(
				m,
				field,
			)

			return true
		}

		*index++

		var inside []protoreflect.Message

		switch {
		case field.IsMap():
			if field.MapValue().Message() == nil {
				continue
			}

			var keys []string

			values := map[string]protoreflect.Message{}

			m.Get(field).Map().Range(func(
				key protoreflect.MapKey,
				value protoreflect.Value,
			) bool {
				keys = append(
					keys,
					key.String(),
				)
				values[key.String()] = value.Message()

				return true
			})

			sort.Strings(keys)

			for _, key := range keys {
				inside = append(
					inside,
					values[key],
				)
			}

		case field.IsList():
			if field.Message() == nil {
				continue
			}

			list := m.Get(field).List()

			for item := range list.Len() {
				inside = append(
					inside,
					list.Get(item).Message(),
				)
			}

		case field.Message() != nil:
			inside = append(
				inside,
				m.Get(field).Message(),
			)
		}

		for _, next := range inside {
			if _Fields(
				next,
				index,
				target,
				apply,
			) {
				return true
			}
		}
	}

	return false
}

// _Varied is the flow base with its field number target emptied (how 0), or set to -1 when
// it is a number (how 1), and whether the field was a number for how 1.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func _Varied(
	base *workflowpb.Flow,
	target,
	how int,
) (*workflowpb.Flow, bool) {
	var (
		flow    = proto.Clone(base).(*workflowpb.Flow)
		index   int
		changed = true
	)

	_Fields(
		flow.ProtoReflect(),
		&index,
		target,
		func(
			m protoreflect.Message,
			field protoreflect.FieldDescriptor,
		) {
			if how == 0 {
				m.Clear(field)

				return
			}

			switch field.Kind() {
			case protoreflect.Int32Kind:
				m.Set(
					field,
					protoreflect.ValueOfInt32(-1),
				)

			case protoreflect.Int64Kind:
				m.Set(
					field,
					protoreflect.ValueOfInt64(-1),
				)

			default:
				changed = false
			}
		},
	)

	return flow, changed
}

// _Emitted checks the property of Read: a flow that Read returns is a flow that Emit
// writes. It returns whether Read returned the flow.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q445, Q453)
func _Emitted(
	t *testing.T,
	flow *workflowpb.Flow,
	what string,
) bool {
	t.Helper()

	sent, err := protojson.Marshal(flow)
	if err != nil {
		return false
	}

	read, err := runtime.Read(sent)
	if err != nil {
		return false
	}

	_, err = runtime.Emit(read)
	if err != nil {
		t.Errorf(
			"%s: Read returned a flow that Emit refuses: %v\n%s",
			what,
			err,
			sent,
		)
	}

	return true
}

// TestRead_EmitWritesTheFlowWithAnyFieldEmptied proves, from the schema, that a flow Read
// returns is a flow Emit writes. The flows EVERY_FLOW hold every message and every member
// of every oneof of workflow.Flow (a test below fails if the schema has a field they do not
// set). Each flow is read once whole, and then once for each field that it sets: with that
// field emptied, and with that field set to -1 when it is a number. So each field Emit reads
// is left empty once (B296, Q453).
//
// The check: go test -race -count=1 -run TestRead_EmitWritesTheFlowWithAnyFieldEmptied
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q445, Q453)
func TestRead_EmitWritesTheFlowWithAnyFieldEmptied(t *testing.T) {
	var accepted, tried int

	for number, text := range EVERY_FLOW {
		var (
			base = new(workflowpb.Flow)
			err  = protojson.Unmarshal(
				[]byte(text),
				base,
			)
		)
		if err != nil {
			t.Fatalf(
				"flow %d does not decode: %v",
				number,
				err,
			)
		}

		if !_Emitted(
			t,
			base,
			fmt.Sprintf(
				"flow %d whole",
				number,
			),
		) {
			t.Fatalf(
				"flow %d must be read whole",
				number,
			)
		}

		var count int

		_Fields(
			base.ProtoReflect(),
			&count,
			-1,
			nil,
		)

		for target := range count {
			for how := range 2 {
				varied, changed := _Varied(
					base,
					target,
					how,
				)
				if !changed {
					continue
				}

				tried++

				if _Emitted(
					t,
					varied,
					fmt.Sprintf(
						"flow %d field %d how %d",
						number,
						target,
						how,
					),
				) {
					accepted++
				}
			}
		}
	}

	if tried < 150 || accepted < tried/10 {
		t.Fatalf(
			"%d flows tried and %d read: the test checks too little",
			tried,
			accepted,
		)
	}
}

// TestRead_EveryFieldOfTheSchemaIsInAFlowOfTheTest proves the flows of the test above set
// every field of every message that workflow.Flow reaches, so a field added to the schema
// fails here until a flow sets it.
//
// The check: go test -race -count=1 -run TestRead_EveryFieldOfTheSchemaIsInAFlowOfTheTest
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func TestRead_EveryFieldOfTheSchemaIsInAFlowOfTheTest(t *testing.T) {
	var (
		want = map[protoreflect.FullName]bool{}
		got  = map[protoreflect.FullName]bool{}
		seen = map[protoreflect.FullName]bool{}
		walk func(message protoreflect.MessageDescriptor)
	)

	walk = func(message protoreflect.MessageDescriptor) {
		if seen[message.FullName()] {
			return
		}

		seen[message.FullName()] = true

		fields := message.Fields()

		for position := range fields.Len() {
			field := fields.Get(position)
			want[field.FullName()] = true

			target := field.Message()
			if field.IsMap() {
				target = field.MapValue().Message()
			}

			if target != nil {
				walk(target)
			}
		}
	}

	walk(new(workflowpb.Flow).ProtoReflect().Descriptor())

	for _, text := range EVERY_FLOW {
		var (
			base = new(workflowpb.Flow)
			err  = protojson.Unmarshal(
				[]byte(text),
				base,
			)
		)
		if err != nil {
			t.Fatal(err)
		}

		var (
			count int
			stop  = -1
		)

		for target := 0; ; target++ {
			var named protoreflect.FieldDescriptor

			index := 0

			if !_Fields(
				base.ProtoReflect(),
				&index,
				target,
				func(
					_ protoreflect.Message,
					field protoreflect.FieldDescriptor,
				) {
					named = field
				},
			) {
				stop = target

				break
			}

			got[named.FullName()] = true
			count++
		}

		if stop != count {
			t.Fatalf(
				"walked %d fields and stopped at %d",
				count,
				stop,
			)
		}
	}

	var missing []string

	for name := range want {
		if !got[name] {
			missing = append(
				missing,
				string(name),
			)
		}
	}

	slices.Sort(missing)

	if len(missing) > 0 {
		t.Fatalf(
			"no flow of the test sets %d fields: %v",
			len(missing),
			missing,
		)
	}
}

// TestRead_EmitWritesARandomFlowOfTheSchema proves the same property on flows that a
// generator fills from the schema: each field of each message is set at random, from a
// small pool of names and of numbers that includes -1 and 0, and a member of each oneof is
// picked or none. The seed is fixed, so the run is the same each time.
//
// The check: go test -race -count=1 -run TestRead_EmitWritesARandomFlowOfTheSchema
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q445, Q453)
func TestRead_EmitWritesARandomFlowOfTheSchema(t *testing.T) {
	var (
		random = rand.New(rand.NewPCG(
			1,
			2,
		))
		accepted int
	)

	const FLOWS = 40000

	for number := range FLOWS {
		flow := new(workflowpb.Flow)

		_Filled(
			flow.ProtoReflect(),
			random,
			0,
		)

		if _Emitted(
			t,
			flow,
			fmt.Sprintf(
				"random flow %d",
				number,
			),
		) {
			accepted++
		}
	}

	if accepted < FLOWS/200 {
		t.Fatalf(
			"%d of %d flows were read: the test checks too little",
			accepted,
			FLOWS,
		)
	}
}

// _Filled sets the fields of m at random.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func _Filled(
	m protoreflect.Message,
	random *rand.Rand,
	depth int,
) {
	const DEEP = 7

	fields := m.Descriptor().Fields()

	for position := range fields.Len() {
		field := fields.Get(position)

		if random.IntN(3) != 0 && field.ContainingOneof() == nil {
			continue
		}

		if field.ContainingOneof() != nil && random.IntN(3) != 0 {
			continue
		}

		switch {
		case field.IsMap():
			if depth > DEEP || random.IntN(2) == 0 {
				continue
			}

			var (
				entries = m.Mutable(field).Map()
				key     = protoreflect.ValueOfString(_Pooled(random)).MapKey()
			)

			if field.MapValue().Message() == nil {
				entries.Set(
					key,
					_Scalar(
						field.MapValue(),
						random,
					),
				)

				continue
			}

			value := entries.NewValue()
			_Filled(
				value.Message(),
				random,
				depth+1,
			)
			entries.Set(
				key,
				value,
			)

		case field.IsList():
			list := m.Mutable(field).List()

			for range random.IntN(3) {
				if field.Message() == nil {
					list.Append(_Scalar(
						field,
						random,
					))

					continue
				}

				if depth > DEEP {
					continue
				}

				item := list.NewElement()
				_Filled(
					item.Message(),
					random,
					depth+1,
				)
				list.Append(item)
			}

		case field.Message() != nil:
			if depth > DEEP {
				continue
			}

			_Filled(
				m.Mutable(field).Message(),
				random,
				depth+1,
			)

		default:
			m.Set(
				field,
				_Scalar(
					field,
					random,
				),
			)
		}
	}
}

// _Pooled is a name from a small pool, so that the names a flow uses meet.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func _Pooled(random *rand.Rand) string {
	pool := []string{"", "f", "g", "x", "h", "K", "A", "pass"}

	return pool[random.IntN(len(pool))]
}

// _Scalar is a value of the kind of field, from the pools.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
func _Scalar(
	field protoreflect.FieldDescriptor,
	random *rand.Rand,
) protoreflect.Value {
	numbers := []int64{-1, 0, 1, 2, 5}

	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(random.IntN(2) == 0)

	case protoreflect.Int32Kind:
		return protoreflect.ValueOfInt32(int32(numbers[random.IntN(len(numbers))]))

	case protoreflect.Int64Kind:
		return protoreflect.ValueOfInt64(numbers[random.IntN(len(numbers))])

	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(float64(numbers[random.IntN(len(numbers))]))

	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte(_Pooled(random)))

	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(0)

	default:
		return protoreflect.ValueOfString(_Pooled(random))
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// EVERY_FLOW lists flows that Read returns and that set, between them, every field of
// every message that workflow.Flow reaches: each kind of value, each member of each oneof,
// each kind of statement.
//
// Revisions:
//   - 2026-11-13 21:20: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	EVERY_FLOW = []string{
		`{
  "functions": [
    {"name": "f", "body": "pass"},
    {"name": "g", "params": ["p", "q"], "statements": {"statement": [{"call": {"function": "f"}}]}}
  ],
  "constants": {
    "K": {"value": {"list": {"items": [{"null": null}, {"bool": true}, {"int": "1"}, {"float": 1.5}, {"string": "s"}, {"bytes": "AA=="}, {"map": {"entries": {"k": {"int": "2"}}}}]}}},
    "C": {"call": {"function": "f"}}
  },
  "args": {"A": {"name": "a", "default": {"int": "1"}}, "B": {"name": "b"}},
  "init": [{"call": {"function": "f"}}, {"text": "X = 1"}],
  "main": {"statement": [
    {"call": {"function": "g", "args": [{"int": "1"}, {"int": "2"}], "result": "r1"}},
    {"call": {"function": "g", "operands": [{"literal": {"int": "1"}}, {"name": "K"}], "result": "r2"}},
    {"call": {"function": "g", "operands": [{"expression": "K"}, {"literal": {"int": "1"}}]}},
    {"call": {"function": "g", "keywords": [{"name": "p", "value": {"literal": {"int": "1"}}}, {"name": "q", "value": {"name": "A"}}]}},
    {"call": {"function": "print", "builtin": true, "args": [{"string": "x"}]}},
    {"spawn": {"binding": "h", "call": {"function": "f"}}},
    {"join": {"bindings": ["h"], "result": "jr"}},
    {"spawn": {"binding": "h2", "call": {"function": "f"}}},
    {"cancel": {"bindings": ["h2"]}},
    {"if": {"condition": {"value": true}, "then": {"call": {"function": "f"}}, "else": {"call": {"function": "f"}}}},
    {"if": {"condition": {"name": "A"}, "then": {"call": {"function": "f"}}}},
    {"if": {"condition": {"call": {"function": "f"}}, "then": {"call": {"function": "f"}}}},
    {"match": {"expression": {"value": "v"}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}], "default": {"call": {"function": "f"}}}},
    {"match": {"expression": {"name": "A"}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}]}},
    {"match": {"expression": {"call": {"function": "f"}}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}]}},
    {"repeat": {"call": {"function": "f"}, "count": 2, "delay_ms": 5}},
    {"retry": {"call": {"function": "f"}, "attempts": 3, "delay_ms": 5}},
    {"sleep": {"duration_ms": 5}},
    {"timeout": {"call": {"function": "f"}, "timeout_ms": 50}},
    {"loop": {"times": {"literal": {"int": "2"}}, "body": {"call": {"function": "f"}}}},
    {"loop": {"times": {"name": "A"}, "body": {"call": {"function": "f"}}}}
  ]}
}`,
		`{
  "functions": [{"name": "f", "body": "pass"}],
  "text": "f()"
}`,
	}
)

// FUZZ_FOUND lists a flow that a fuzz run of FuzzRead found Read returning and Emit refusing:
// an if with no condition (the tester's run of 180 s failed on it), a loop with no count, and
// a count that is an expression.
//
// Revisions:
//   - 2026-11-13 21:23: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	FUZZ_FOUND = []string{
		`{"functions": [{"name": "f", "body": "A000"}, {"name": "A", "params": ["A"], "body": "A000"}], "main": {"statement": [{"if": {"then": {"call": {"function": "f"}}}}]}}`,
		`{"functions": [{"name": "g", "body": "pass"}], "main": {"statement": [{"if": {"condition": {}, "then": {"call": {"function": "g"}}}}]}}`,
		`{"functions": [{"name": "g", "body": "pass"}], "main": {"statement": [{"loop": {"body": {"call": {"function": "g"}}}}]}}`,
		`{"functions": [{"name": "g", "body": "pass"}], "main": {"statement": [{"loop": {"times": {}, "body": {"call": {"function": "g"}}}}]}}`,
		`{"functions": [{"name": "g", "body": "pass"}], "main": {"statement": [{"loop": {"times": {"expression": ""}, "body": {"call": {"function": "g"}}}}]}}`,
		`{"functions": [{"name": "g", "body": "pass"}], "main": {"statement": [{"loop": {"times": {"expression": "2"}, "body": {"call": {"function": "g"}}}}]}}`,
	}
)

// END GOLDEN
