package tests

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
)

// TestNilSweep_buf calls the plugin's methods on a nil and a zero receiver, and
// every builtin of its module with each nil or odd argument shape a host or a
// plugin author could pass (C87, Q2, Q85). A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNilSweep_buf(t *testing.T) {
	t.Run(
		"nil and zero receivers",
		func(t *testing.T) {
			nilsweep.Methods(
				t,
				"buf",
			)
		},
	)

	members := nilsweep.Members(
		t,
		"buf",
	)

	for _, name := range slices.Sorted(maps.Keys(members)) {
		t.Run(
			name,
			func(t *testing.T) {
				nilsweep.Run(
					t,
					members[name],
				)
			},
		)
	}

	var (
		nilBuffer  *buffer.Buffer
		nilReader  *buffer.Reader
		zeroBuffer = new(buffer.Buffer)
		zeroReader = new(buffer.Reader)
		args       = starlark.Tuple{starlark.MakeInt(1)}
	)

	nilsweep.Rows(
		t,
		[]nilsweep.Row{
			{
				Name: "nil Buffer value methods",
				Call: func() error {
					_ = nilBuffer.String()
					_ = nilBuffer.Type()
					_ = nilBuffer.Truth()
					_, _ = nilBuffer.Hash()
					_ = nilBuffer.Len()
					_ = nilBuffer.AttrNames()
					nilBuffer.Freeze()

					return nil
				},
			},
			{
				Name: "zero Buffer value methods",
				Call: func() error {
					_ = zeroBuffer.String()
					_ = zeroBuffer.Truth()
					_ = zeroBuffer.Len()

					return nil
				},
			},
			{
				Name: "Buffer Index out of range",
				Call: func() error {
					for _, index := range []int{-1, 0, 1} {
						_ = zeroBuffer.Index(index)
						_ = nilBuffer.Index(index)
					}

					return nil
				},
			},
			{
				Name: "nil Reader value methods",
				Call: func() error {
					_ = nilReader.String()
					_ = nilReader.Type()
					_ = nilReader.Truth()
					_, _ = nilReader.Hash()
					_ = nilReader.Len()
					_ = nilReader.AttrNames()
					nilReader.Freeze()

					return nil
				},
			},
			{
				Name: "zero Reader value methods",
				Call: func() error {
					_ = zeroReader.String()
					_ = zeroReader.Truth()
					_ = zeroReader.Len()

					return nil
				},
			},
			{
				Name: "Reader Index out of range",
				Call: func() error {
					for _, index := range []int{-1, 0, 1} {
						_ = zeroReader.Index(index)
						_ = nilReader.Index(index)
					}

					return nil
				},
			},
			{
				Name: "Attr of an unknown name",
				Call: func() error {
					for _, value := range []starlark.HasAttrs{
						nilBuffer,
						zeroBuffer,
						nilReader,
						zeroReader,
					} {
						attr, err := value.Attr("")
						if err != nil || attr != nil {
							return errors.New("attr")
						}
					}

					return nil
				},
			},
			{
				Name: "every method of a Buffer on a nil Buffer",
				Call: func() error {
					return _CallEvery(
						nilBuffer,
						buffer.BUFFER_METHODS,
						args,
					)
				},
				Want: buf.ErrFrozen,
			},
			{
				Name: "every method of a Reader on a nil Reader",
				Call: func() error {
					return _CallEvery(
						nilReader,
						buffer.READER_METHODS,
						args,
					)
				},
				Want: buf.ErrFrozen,
			},
			{
				Name: "every method of a Buffer, unbound",
				Call: func() error {
					return _CallEvery(
						nil,
						buffer.BUFFER_METHODS,
						args,
					)
				},
				Want: buf.ErrFrozen,
			},
			{
				Name: "every method of a Reader, unbound",
				Call: func() error {
					return _CallEvery(
						nil,
						buffer.READER_METHODS,
						args,
					)
				},
				Want: buf.ErrFrozen,
			},
			{
				Name: "every method bound to the wrong type",
				Call: func() error {
					err := _CallEvery(
						starlark.String("x"),
						buffer.BUFFER_METHODS,
						args,
					)
					if !errors.Is(
						err,
						buf.ErrFrozen,
					) {
						return err
					}

					return _CallEvery(
						zeroBuffer,
						buffer.READER_METHODS,
						args,
					)
				},
				Want: buf.ErrFrozen,
			},
			{
				Name: "every method of a zero Buffer and a zero Reader",
				Call: func() error {
					_ = _CallEvery(
						zeroBuffer,
						buffer.BUFFER_METHODS,
						args,
					)

					return _CallEvery(
						zeroReader,
						buffer.READER_METHODS,
						args,
					)
				},
			},
		},
	)
}

// _CallEvery binds each method to receiver (nil: unbound) and calls it with
// args on a nil thread. It returns the first ErrFrozen wrapped error it meets,
// or nil. A panic is not caught here.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _CallEvery(
	receiver starlark.Value,
	methods map[string]*starlark.Builtin,
	args starlark.Tuple,
) error {
	var first error

	for _, name := range slices.Sorted(maps.Keys(methods)) {
		method := methods[name]
		if receiver != nil {
			method = method.BindReceiver(receiver)
		}

		_, err := method.CallInternal(
			nil,
			args,
			nil,
		)
		if errors.Is(
			err,
			buf.ErrFrozen,
		) && first == nil {
			first = err
		}
	}

	return first
}
