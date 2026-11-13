package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestCheckPadding_Table is RFC 4648 sections 3.3 and 4 and 6: no padding or the
// exact padding that fills the group, and no CR or LF.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCheckPadding_Table(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		group int
		want  string
		ok    bool
	}{
		{
			"empty",
			"",
			4,
			"",
			true,
		},
		{
			"no padding",
			"Zg",
			4,
			"Zg",
			true,
		},
		{
			"exact two",
			"Zg==",
			4,
			"Zg",
			true,
		},
		{
			"exact one",
			"Zm8=",
			4,
			"Zm8",
			true,
		},
		{
			"whole group",
			"Zm9v",
			4,
			"Zm9v",
			true,
		},
		{
			"too few",
			"Zg=",
			4,
			"Zg",
			false,
		},
		{
			"too many",
			"Zg===",
			4,
			"Zg",
			false,
		},
		{
			"on a whole group",
			"Zm9v=",
			4,
			"Zm9v",
			false,
		},
		{
			"only padding",
			"=",
			4,
			"",
			false,
		},
		{
			"base32 exact",
			"MY======",
			8,
			"MY",
			true,
		},
		{
			"base32 too many",
			"MY=======",
			8,
			"MY",
			false,
		},
		{
			"base32 only padding",
			"========",
			8,
			"",
			false,
		},
		{
			"lf",
			"Zm9v\n",
			4,
			"",
			false,
		},
		{
			"cr",
			"Zm\r9v",
			4,
			"",
			false,
		},
		{
			"lf after padding",
			"Zg==\n",
			4,
			"",
			false,
		},
	}

	for _, test := range cases {
		t.Run(
			test.name,
			func(t *testing.T) {
				got, ok := unpack.CheckPadding(
					test.text,
					test.group,
				)
				if ok != test.ok {
					t.Fatalf(
						"CheckPadding(%q, %d) ok = %v, want %v",
						test.text,
						test.group,
						ok,
						test.ok,
					)
				}

				if ok && got != test.want {
					t.Fatalf(
						"CheckPadding(%q, %d) = %q, want %q",
						test.text,
						test.group,
						got,
						test.want,
					)
				}
			},
		)
	}
}
