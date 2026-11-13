package tests

import (
	"context"
	"io"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// BenchmarkShown measures a script that shows values, which is what the depth
// check of change C47 adds work to. Run it on the base and on the change, as
//
//	go test -run '^$' -bench Shown -benchmem -count 6 ./v1/testing
//
// Each row runs one script of the same text on both sides.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: logs to io.Discard through WithLog, WithLogger being dropped (C121, Q340)
func BenchmarkShown(b *testing.B) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "str of 10000 small ints",
			body: "    for i in range(10000):\n        str(i)\n",
		},
		{
			name: "print of 10000 lines of three scalars",
			body: "    for i in range(10000):\n" +
				"        print(i, \"a\", None)\n",
		},
		{
			name: "str of 1000 small lists",
			body: "    x = [1, 2, 3]\n    for i in range(1000):\n        str(x)\n",
		},
		{
			name: "str of one flat list of 100000 ints",
			body: "    x = list(range(100000))\n    str(x)\n",
		},
		{
			name: "str of 1000 dicts of 10",
			body: "    x = [{\"a\": 1, \"b\": 2, \"c\": 3, \"d\": 4, \"e\": 5, " +
				"\"f\": 6, \"g\": 7, \"h\": 8, \"i\": 9, \"j\": 10} " +
				"for i in range(1000)]\n    str(x)\n",
		},
		{
			name: "str of a list nested 1000",
			body: "    x = []\n    for i in range(1000):\n" +
				"        x = [x]\n    str(x)\n",
		},
	}

	for _, tc := range tests {
		b.Run(
			tc.name,
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
					Entry: "bench.star",
					Text:  []byte("def main():\n" + tc.body),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					run, err := runtime.Start(
						context.Background(),
						built,
						runtime.WithLog(io.Discard),
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = run.Wait()
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
