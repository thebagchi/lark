package tests

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch/internal/watch"
)

const (
	// FAST is a poll interval short enough that a test waits a few
	// milliseconds, and WAIT how long a test waits for something that must
	// happen, which only a broken watch makes it spend.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FAST = time.Millisecond
	WAIT = 5 * time.Second

	// PRIVATE is the mode a test file is made with.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PRIVATE = 0o640
)

// _Fixed is a source that always returns used of total.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Fixed(
	used uint64,
	total uint64,
) func() (*common.MemoryReading, error) {
	return func() (*common.MemoryReading, error) {
		return &common.MemoryReading{
			Used:  used,
			Total: total,
		}, nil
	}
}

// _Write makes a file under dir and returns its path.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Write(
	t *testing.T,
	dir string,
	name string,
	text string,
) string {
	t.Helper()

	var (
		path = filepath.Join(
			dir,
			name,
		)
		err = os.MkdirAll(
			filepath.Dir(path),
			0o750,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		path,
		[]byte(text),
		PRIVATE,
	)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// TestStart_TripsAtThePercent proves the trip rule with injected readings, so
// no real memory is used. The limit is 95 percent by default, the edge counts,
// and a reading that fails or has no total never trips.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 18:26: wait up to WAIT for a trip that must come (C104)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestStart_TripsAtThePercent(t *testing.T) {
	cases := []struct {
		name    string
		percent float64
		source  func() (*common.MemoryReading, error)
		trips   bool
	}{
		{
			"94 percent of 100 is under the default",
			0,
			_Fixed(
				94,
				100,
			),
			false,
		},
		{
			"95 percent of 100 is the edge and trips",
			0,
			_Fixed(
				95,
				100,
			),
			true,
		},
		{
			"96 percent trips",
			0,
			_Fixed(
				96,
				100,
			),
			true,
		},
		{
			"a set percent replaces the default",
			50,
			_Fixed(
				50,
				100,
			),
			true,
		},
		{
			"no total is not a trip",
			0,
			_Fixed(
				5,
				0,
			),
			false,
		},
		{
			"a failed reading is not a trip",
			0,
			func() (*common.MemoryReading, error) {
				return nil, watch.ErrUnavailable
			},
			false,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					tripped = make(
						chan error,
						4,
					)
					stop = watch.Start(
						t.Context(),
						&common.MemoryWatch{
							Percent:  item.percent,
							Interval: FAST,
							Source:   item.source,
						},
						func(err error) {
							tripped <- err
						},
					)
				)

				wait := 50 * FAST
				if item.trips {
					wait = WAIT
				}

				select {
				case err := <-tripped:
					if !item.trips {
						t.Fatalf(
							"tripped with %v, want no trip",
							err,
						)
					}

					if !errors.Is(
						err,
						watch.ErrLimit,
					) ||
						err.Error() != "memory limit reached" {
						t.Fatalf(
							"got %q, want memory limit reached",
							err,
						)
					}

				case <-time.After(wait):
					if item.trips {
						t.Fatal("did not trip")
					}
				}

				stop()

				if len(tripped) > 1 {
					t.Fatalf(
						"tripped %d times, want once",
						len(tripped)+1,
					)
				}
			},
		)
	}
}

// TestStart_GraceIsHalfASecond proves the grace default is 500 ms.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStart_GraceIsHalfASecond(t *testing.T) {
	if common.WATCH_GRACE != 500*time.Millisecond {
		t.Fatalf(
			"grace is %v, want 500ms",
			common.WATCH_GRACE,
		)
	}
}

// TestStart_Abandon proves the grace: Abandon runs once, after the grace, when
// the run does not end; it does not run when the run ends inside the grace,
// when nothing tripped, or when no Abandon is set.
//
// The test waits on events, not on the clock. A case that must abandon waits up
// to WAIT for the call. The "ends inside the grace" case uses a grace of WAIT
// and stops at once, so a loaded machine cannot push the stop past the grace;
// stop joins the watch goroutine, so one look at the channel after stop is
// final. A case that must not abandon sleeps through three graces, which a slow
// machine only makes longer, never wrong.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 17:42: wait on events, not on short sleeps (C104, B183)
//   - 2026-11-07 18:26: stamp the trip inside the callback, where the grace starts
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestStart_Abandon(t *testing.T) {
	const (
		GRACE = 100 * time.Millisecond
	)

	cases := []struct {
		name    string
		source  func() (*common.MemoryReading, error)
		set     bool
		grace   time.Duration
		ends    bool
		trips   bool
		abandon bool
	}{
		{
			"a run that never ends is abandoned",
			_Fixed(
				99,
				100,
			),
			true,
			GRACE,
			false,
			true,
			true,
		},
		{
			"a run that ends inside the grace is not",
			_Fixed(
				99,
				100,
			),
			true,
			WAIT,
			true,
			true,
			false,
		},
		{
			"no trip, no abandon",
			_Fixed(
				1,
				100,
			),
			true,
			GRACE,
			false,
			false,
			false,
		},
		{
			"no Abandon set leaves the process alone",
			_Fixed(
				99,
				100,
			),
			false,
			GRACE,
			false,
			true,
			false,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					abandoned = make(
						chan error,
						2,
					)
					tripped = make(
						chan time.Time,
						2,
					)
					settings = &common.MemoryWatch{
						Interval: FAST,
						Grace:    item.grace,
						Source:   item.source,
					}
				)

				if item.set {
					settings.Abandon = func(err error) {
						abandoned <- err
					}
				}

				stop := watch.Start(
					t.Context(),
					settings,
					func(error) {
						// The grace starts after this returns, so this
						// time is never later than that start.
						tripped <- time.Now()
					},
				)
				defer stop()

				var began time.Time

				if item.trips {
					select {
					case began = <-tripped:

					case <-time.After(WAIT):
						t.Fatal("did not trip")
					}
				}

				var late bool

				switch {
				case item.ends:
					stop()

					select {
					case <-abandoned:
						late = true

					default:
					}

				case item.abandon:
					select {
					case err := <-abandoned:
						late = true

						if !errors.Is(
							err,
							watch.ErrLimit,
						) {
							t.Fatalf(
								"abandoned with %v",
								err,
							)
						}

						waited := time.Since(began)
						if waited < item.grace {
							t.Fatalf(
								"abandoned after %v, grace %v",
								waited,
								item.grace,
							)
						}

					case <-time.After(WAIT):
					}

				default:
					select {
					case <-abandoned:
						late = true

					case <-time.After(3 * item.grace):
					}
				}

				if late != item.abandon {
					t.Fatalf(
						"abandoned %v, want %v",
						late,
						item.abandon,
					)
				}
			},
		)
	}
}

// TestStart_OffCostsNothing proves a nil watch starts no goroutine and
// allocates nothing: the default for every run.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStart_OffCostsNothing(t *testing.T) {
	var (
		calls atomic.Int64
		trip  = func(error) {
			calls.Add(1)
		}
		before = runtime.NumGoroutine()
		allocs = testing.AllocsPerRun(
			100,
			func() {
				watch.Start(
					t.Context(),
					nil,
					trip,
				)()
			},
		)
	)

	if allocs != 0 {
		t.Fatalf(
			"a nil watch allocated %v times, want 0",
			allocs,
		)
	}

	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf(
			"goroutines %d -> %d, want no new one",
			before,
			after,
		)
	}

	if calls.Load() != 0 {
		t.Fatal("a nil watch called trip")
	}
}

// TestMachine reads MemTotal less MemAvailable from a meminfo file, and
// refuses a file without them, naming the file.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMachine(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		missing bool
		used    uint64
		total   uint64
		failure string
	}{
		{
			name:  "total and available",
			text:  "MemTotal:       1000 kB\nMemFree: 1 kB\nMemAvailable:    250 kB\n",
			used:  750 * 1024,
			total: 1000 * 1024,
		},
		{
			name:    "no MemAvailable",
			text:    "MemTotal: 1000 kB\n",
			failure: "no usable MemTotal",
		},
		{
			name:    "available above total",
			text:    "MemTotal: 10 kB\nMemAvailable: 20 kB\n",
			failure: "no usable MemTotal",
		},
		{
			name:    "a number that wraps",
			text:    "MemTotal: 18446744073709551615 kB\nMemAvailable: 1 kB\n",
			failure: "no usable MemTotal",
		},
		{
			name:    "no file",
			missing: true,
			failure: "cannot read",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				path := filepath.Join(
					t.TempDir(),
					"meminfo",
				)

				if !item.missing {
					path = _Write(
						t,
						filepath.Dir(path),
						"meminfo",
						item.text,
					)
				}

				got, err := watch.Machine(path)
				if item.failure != "" {
					if !errors.Is(
						err,
						watch.ErrUnavailable,
					) ||
						!strings.Contains(
							err.Error(),
							item.failure,
						) ||
						!strings.Contains(
							err.Error(),
							path,
						) {
						t.Fatalf(
							"got %v, want %q naming %s",
							err,
							item.failure,
							path,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				if got.Used != item.used || got.Total != item.total {
					t.Fatalf(
						"got %+v, want used %d total %d",
						got,
						item.used,
						item.total,
					)
				}
			},
		)
	}
}

// _System makes a cgroup tree with the given files under /job, and a meminfo
// file beside it that reads 1000 kB total and 200 kB available. It returns the
// meminfo path, the mount and the self file.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _System(
	t *testing.T,
	files map[string]string,
) (string, string, string) {
	t.Helper()

	root, self := _Tree(
		t,
		"/job",
		files,
	)

	return _Write(
		t,
		root,
		"meminfo",
		"MemTotal: 1000 kB\nMemAvailable: 200 kB\n",
	), root, self
}

// _Tree makes a cgroup v2 tree under a temporary directory, with the process in
// place, and returns the mount and the self file.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Tree(
	t *testing.T,
	place string,
	files map[string]string,
) (string, string) {
	t.Helper()

	var (
		root = t.TempDir()
		self = _Write(
			t,
			root,
			"self",
			"0::"+place+"\n",
		)
	)

	_Write(
		t,
		root,
		watch.MARKER,
		"cpu memory\n",
	)

	for name, text := range files {
		_Write(
			t,
			root+place,
			name,
			text,
		)
	}

	return root, self
}

// TestCgroup covers the limit cases: "max" is no limit, no memory.max file is
// no limit, a number is a limit, and the use is memory.current less
// inactive_file (page cache subtracted), never below zero.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCgroup(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		limited bool
		used    uint64
		total   uint64
	}{
		{
			name:  "max is no limit",
			files: map[string]string{"memory.max": "max\n"},
		},
		{
			name:  "no memory.max is no limit",
			files: map[string]string{},
		},
		{
			name: "a limit, with page cache taken off the use",
			files: map[string]string{
				"memory.max":     "1000\n",
				"memory.current": "900\n",
				"memory.stat":    "anon 1\ninactive_file 400\nactive_file 7\n",
			},
			limited: true,
			used:    500,
			total:   1000,
		},
		{
			name: "no inactive_file key subtracts nothing",
			files: map[string]string{
				"memory.max":     "1000\n",
				"memory.current": "900\n",
				"memory.stat":    "anon 1\n",
			},
			limited: true,
			used:    900,
			total:   1000,
		},
		{
			name: "inactive_file above current leaves zero",
			files: map[string]string{
				"memory.max":     "1000\n",
				"memory.current": "100\n",
				"memory.stat":    "inactive_file 400\n",
			},
			limited: true,
			used:    0,
			total:   1000,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					root, self = _Tree(
						t,
						"/job",
						item.files,
					)
					got, err = watch.Cgroup(
						root,
						self,
					)
					limited = got != nil
				)
				if err != nil {
					t.Fatal(err)
				}

				if limited != item.limited ||
					(limited && (got.Used != item.used ||
						got.Total != item.total)) {
					t.Fatalf(
						"got %+v limited %v, want %d of %d limited %v",
						got,
						limited,
						item.used,
						item.total,
						item.limited,
					)
				}
			},
		)
	}
}

// TestRead_TheTighterShareWins proves the limit is the tighter of the machine
// and the cgroup, in both orders, and that a cgroup with no limit leaves the
// machine.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRead_TheTighterShareWins(t *testing.T) {
	cases := []struct {
		name   string
		cgroup map[string]string
		used   uint64
		total  uint64
	}{
		{
			name: "cgroup fuller than the machine",
			cgroup: map[string]string{
				"memory.max":     "2000\n",
				"memory.current": "1900\n",
				"memory.stat":    "inactive_file 0\n",
			},
			used:  1900,
			total: 2000,
		},
		{
			name: "machine fuller than the cgroup",
			cgroup: map[string]string{
				"memory.max":     "1000000000\n",
				"memory.current": "10\n",
				"memory.stat":    "inactive_file 0\n",
			},
			used:  800 * 1024,
			total: 1000 * 1024,
		},
		{
			name:   "cgroup without a limit",
			cgroup: map[string]string{"memory.max": "max\n"},
			used:   800 * 1024,
			total:  1000 * 1024,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					meminfo, root, self = _System(
						t,
						item.cgroup,
					)
					got, err = watch.Read(
						meminfo,
						root,
						self,
					)
				)
				if err != nil {
					t.Fatal(err)
				}

				if got.Used != item.used || got.Total != item.total {
					t.Fatalf(
						"got %+v, want used %d total %d",
						got,
						item.used,
						item.total,
					)
				}
			},
		)
	}
}

// TestCheck proves the watch refuses a system that does not offer what it
// reads, and says what it could not read: no meminfo, no cgroup v2 mount, no
// cgroup line. A good tree passes.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck(t *testing.T) {
	const (
		GOOD = "MemTotal: 1000 kB\nMemAvailable: 200 kB\n"
	)

	cases := []struct {
		name  string
		build func(t *testing.T) (string, string, string)
		names string
	}{
		{
			name: "a good tree",
			build: func(t *testing.T) (string, string, string) {
				t.Helper()

				root, self := _Tree(
					t,
					"/job",
					map[string]string{"memory.max": "max\n"},
				)

				return _Write(
					t,
					root,
					"meminfo",
					GOOD,
				), root, self
			},
		},
		{
			name: "no meminfo",
			build: func(t *testing.T) (string, string, string) {
				t.Helper()

				root, self := _Tree(
					t,
					"/job",
					nil,
				)

				return filepath.Join(
					root,
					"no-meminfo",
				), root, self
			},
			names: "no-meminfo",
		},
		{
			name: "no cgroup v2 mount",
			build: func(t *testing.T) (string, string, string) {
				t.Helper()

				dir := t.TempDir()

				return _Write(
						t,
						dir,
						"meminfo",
						GOOD,
					), dir, _Write(
						t,
						dir,
						"self",
						"0::/job\n",
					)
			},
			names: watch.MARKER,
		},
		{
			name: "no cgroup line",
			build: func(t *testing.T) (string, string, string) {
				t.Helper()

				meminfo, root, _ := _System(
					t,
					nil,
				)

				return meminfo, root, _Write(
					t,
					root,
					"self-v1",
					"12:memory:/job\n",
				)
			},
			names: "self-v1",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					meminfo, root, self = item.build(t)
					err                 = watch.Check(
						meminfo,
						root,
						self,
					)
				)
				if item.names == "" {
					if err != nil {
						t.Fatal(err)
					}

					return
				}

				if !errors.Is(
					err,
					watch.ErrUnavailable,
				) ||
					!strings.Contains(
						err.Error(),
						item.names,
					) {
					t.Fatalf(
						"got %v, want ErrUnavailable naming %q",
						err,
						item.names,
					)
				}
			},
		)
	}
}
