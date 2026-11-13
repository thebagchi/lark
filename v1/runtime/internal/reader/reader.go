// Package reader is internal, not public: it reads flow JSON for runtime.Read and
// refuses what lark cannot run.
//
// Revisions:
//   - 2026-11-13 17:37: initial creation, moved from cmd/lark/internal/command (C133, Q441,
//     Q444, Q447)
package reader

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// NULLED is the hint that goes with the refusal of a JSON null where a value
	// stands.
	//
	// Revisions:
	//   - 2026-11-13 12:47: initial creation (C132, Q439)
	//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
	NULLED = "(a value is an object named for its kind, such as {\"null\": null})"

	// VALUE is the full name of the message a null must not stand for.
	//
	// Revisions:
	//   - 2026-11-13 12:47: initial creation (C132, Q439)
	//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
	VALUE = "workflow.Value"

	// NULL is the name of the field of a value that holds null.
	//
	// Revisions:
	//   - 2026-11-13 12:47: initial creation (C132, Q439)
	//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
	NULL = "null"

	// PLAIN is the words of the decoder when a flow holds a plain JSON value where a
	// value of a kind stands.
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q422)
	//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
	PLAIN = "unexpected token"

	// TAGGED is the hint that goes with the refusal of a flow of plain values.
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q422)
	//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
	TAGGED = "(a value is an object named for its kind, " +
		"such as {\"string\": \"x\"} or {\"int\": \"3\"})"
)

// _FlowError is a flow that Read will not read: the words that are shown, cut and
// with their hint, and what refused it. It matches runtime.ErrForm, the flow that
// has no form, and the decoder's own error, so a caller names the cause with
// errors.Is whatever the words are.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q4, Q439)
//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to package reader; the doc names Read
//     (C133, Q441, Q447)
type _FlowError struct {
	text  string
	cause error
}

// Error is the words that are shown.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q4, Q439)
//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to reader (C133, Q441, Q447)
func (r *_FlowError) Error() string {
	return r.text
}

// Unwrap is ErrForm and the cause.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q4, Q439)
//   - 2026-11-13 17:37: moved from cmd/lark/internal/command to package reader; ErrForm is
//     graph.ErrForm, the value runtime.ErrForm forwards (C133, Q441, Q447)
func (r *_FlowError) Unwrap() []error {
	return []error{
		graph.ErrForm,
		r.cause,
	}
}

// Nulled finds a JSON null that stands for a value, which protojson reads as no
// value at all: an argument default that is null reads as no default, so a run
// that took None takes none, and a constant that is null reads as no constant.
// It also finds a null kind that holds anything but null, which protojson reads as
// null (Q439). It walks the tokens of the JSON by the schema, so a null in a field that
// is no value (a body that is null) is left to protojson, which reads it as unset.
//
// The walk reads one token at a time and holds no copy of the input, so what it holds
// is the path of the schema it is in. Against the tree it replaced it takes about 20%
// more time and about 2.5 times less memory for a flow of many small values (Q458). Its
// depth is bounded by common.DEPTH messages (trail.ErrDeep), the bound of the decoder
// (Q2). Text that is no JSON is an error, and so is data after the value.
//
// The path in the error is made of the names of the schema, never of the input, and is
// cut to 100 bytes (Q4): a flow nests as deep as the decoder allows.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q439)
//   - 2026-11-13 19:32: moved from cmd/lark/internal/command to reader; reads the tokens of the
//     JSON, with no tree (C133, Q2, Q441, Q447, Q458)
func Nulled(src []byte) error {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()

	err := _Message(
		dec,
		new(workflowpb.Flow).ProtoReflect().Descriptor(),
		"flow",
		0,
	)
	if err != nil {
		return err
	}

	_, err = dec.Token()
	if errors.Is(
		err,
		io.EOF,
	) {
		return nil
	}

	if err != nil {
		return err
	}

	return errors.New("data after the value")
}

// _Walked checks the members of one JSON object against the fields of message. The
// opening brace is read. A member is found by its JSON name. protojson takes the name in
// the .proto too, and the two differ for a field with an underscore (delay_ms), but no
// such field holds a value or leads to one, so a member under the other name holds no
// null this walk must see (a test keeps it so). A member that is no field is skipped.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q439)
//   - 2026-11-13 19:32: moved from cmd/lark/internal/command to reader; reads the tokens of the
//     JSON, with no tree (C133, Q2, Q441, Q447)
func _Walked(
	dec *json.Decoder,
	message protoreflect.MessageDescriptor,
	path string,
	depth int,
) error {
	if depth > common.DEPTH {
		return fmt.Errorf(
			"%s: %w",
			unpack.ShortText(path),
			trail.ErrDeep,
		)
	}

	for dec.More() {
		name, err := dec.Token()
		if err != nil {
			return err
		}

		var (
			key, _ = name.(string)
			field  = message.Fields().ByJSONName(key)
		)

		if field == nil {
			err = _Skipped(dec)
			if err != nil {
				return err
			}

			continue
		}

		here := path + "." + string(field.Name())

		if message.FullName() == VALUE && field.Name() == NULL {
			err = _Nothing(
				dec,
				here,
			)
		} else {
			err = _Fielded(
				dec,
				field,
				here,
				depth,
			)
		}

		if err != nil {
			return err
		}
	}

	_, err := dec.Token()

	return err
}

// _Nothing reads the member of a null kind. It must be null (Q439).
//
// Revisions:
//   - 2026-11-13 19:32: initial creation (C133, Q439)
func _Nothing(
	dec *json.Decoder,
	path string,
) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	if token != nil {
		return fmt.Errorf(
			"%s: null holds more than null, which is out of range %s",
			unpack.ShortText(path),
			NULLED,
		)
	}

	return nil
}

// _Fielded checks the JSON of one field: a null where a value stands, and the
// messages inside it. A field that holds no message is skipped.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q439)
//   - 2026-11-13 19:32: moved from cmd/lark/internal/command to reader; reads the tokens of the
//     JSON, with no tree (C133, Q2, Q441, Q447)
func _Fielded(
	dec *json.Decoder,
	field protoreflect.FieldDescriptor,
	path string,
	depth int,
) error {
	if field.Message() == nil {
		return _Skipped(dec)
	}

	switch {
	case field.IsMap():
		return _Entries(
			dec,
			field.MapValue().Message(),
			path+"{}",
			depth,
		)

	case field.IsList():
		return _Items(
			dec,
			field.Message(),
			path,
			depth,
		)

	default:
		return _Message(
			dec,
			field.Message(),
			path,
			depth,
		)
	}
}

// _Entries checks the values of a JSON object that stands for a map of message.
// Anything else (a null, which protojson reads as an empty map) holds no value to find.
//
// Revisions:
//   - 2026-11-13 19:32: initial creation (C133, Q2, Q439)
func _Entries(
	dec *json.Decoder,
	message protoreflect.MessageDescriptor,
	path string,
	depth int,
) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	if token != json.Delim('{') {
		return _Closed(
			dec,
			token,
		)
	}

	for dec.More() {
		_, err = dec.Token()
		if err != nil {
			return err
		}

		err = _Message(
			dec,
			message,
			path,
			depth,
		)
		if err != nil {
			return err
		}
	}

	_, err = dec.Token()

	return err
}

// _Items checks the values of a JSON array that stands for a list of message.
// Anything else holds no value to find.
//
// Revisions:
//   - 2026-11-13 19:32: initial creation (C133, Q2, Q439)
func _Items(
	dec *json.Decoder,
	message protoreflect.MessageDescriptor,
	path string,
	depth int,
) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	if token != json.Delim('[') {
		return _Closed(
			dec,
			token,
		)
	}

	for index := 0; dec.More(); index++ {
		err = _Message(
			dec,
			message,
			path+"["+strconv.Itoa(index)+"]",
			depth,
		)
		if err != nil {
			return err
		}
	}

	_, err = dec.Token()

	return err
}

// _Message checks one JSON value that stands for message: a null is refused when
// the message is a value, and an object is walked. Any other JSON is left to the
// decoder.
//
// Revisions:
//   - 2026-11-13 12:47: initial creation (C132, Q439)
//   - 2026-11-13 19:32: moved from cmd/lark/internal/command to reader; reads the tokens of the
//     JSON, with no tree (C133, Q2, Q441, Q447)
func _Message(
	dec *json.Decoder,
	message protoreflect.MessageDescriptor,
	path string,
	depth int,
) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	if token == nil && message.FullName() == VALUE {
		return fmt.Errorf(
			"%s: a null where a value stands %s",
			unpack.ShortText(path),
			NULLED,
		)
	}

	if token != json.Delim('{') {
		return _Closed(
			dec,
			token,
		)
	}

	return _Walked(
		dec,
		message,
		path,
		depth+1,
	)
}

// _Skipped reads one JSON value and nothing of it.
//
// Revisions:
//   - 2026-11-13 19:32: initial creation (C133, Q2)
func _Skipped(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	return _Closed(
		dec,
		token,
	)
}

// _Closed reads what is left of the value that token began: to the end of an object or
// an array, and nothing for a scalar. It counts the brackets, so it holds no recursion.
//
// Revisions:
//   - 2026-11-13 19:32: initial creation (C133, Q2)
func _Closed(
	dec *json.Decoder,
	token json.Token,
) error {
	open, ok := token.(json.Delim)
	if !ok || open == ']' || open == '}' {
		return nil
	}

	for level := 1; level > 0; {
		next, err := dec.Token()
		if err != nil {
			return err
		}

		held, ok := next.(json.Delim)
		if !ok {
			continue
		}

		if held == '{' || held == '[' {
			level++
		} else {
			level--
		}
	}

	return nil
}

// Read is the flow that the JSON in data holds, once it is a flow lark can run. The
// decoder reads it (protojson), then Nulled refuses a null where a value stands, then
// Check refuses a flow that has no script. A refusal of the first two matches ErrForm
// and the decoder's own error, and its words are cut to 100 bytes (Q4). A flow of
// plain values, written before values were tagged, is refused with a hint that says
// what a value is (Q433). The error of Check is returned as Check gives it. No flow
// comes back with an error.
//
// Read puts no bound on len(data). The decoders bound the depth of what they read.
//
// Revisions:
//   - 2026-11-13 17:37: initial creation, from Generated in cmd/lark/internal/command (C133, Q4,
//     Q433, Q439, Q441, Q444, Q445)
func Read(data []byte) (*workflowpb.Flow, error) {
	var (
		flow = new(workflowpb.Flow)
		err  = protojson.Unmarshal(
			data,
			flow,
		)
	)
	if err != nil {
		return nil, _Refused(
			data,
			err,
		)
	}

	err = Nulled(data)
	if err != nil {
		return nil, &_FlowError{
			text:  err.Error(),
			cause: err,
		}
	}

	err = graph.Check(flow)
	if err != nil {
		return nil, err
	}

	return flow, nil
}

// _Refused is the error for the data the decoder refused: its words cut to 100 bytes, with
// the hint for a flow of plain values when the decoder met a token where an object
// stands. Data that is empty or blank has no token, so it has the decoder's words alone
// (Q451).
//
// Revisions:
//   - 2026-11-13 17:37: initial creation, from Generated in cmd/lark/internal/command
//     (C133, Q4, Q433, Q451)
func _Refused(
	data []byte,
	cause error,
) error {
	var hint string

	if len(bytes.TrimSpace(data)) > 0 && strings.Contains(
		cause.Error(),
		PLAIN,
	) {
		hint = " " + TAGGED
	}

	return &_FlowError{
		text:  unpack.ShortText(cause.Error()) + hint,
		cause: cause,
	}
}
