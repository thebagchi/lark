package observe

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// Ledger is the graph of one run with no live graph, folded one thread at a
// time and drawn once the run ends. It draws the graph a Recorder draws, with
// no lock that every thread of the run waits for.
//
// Each thread's reports fold into its own _Lane, under that lane's own lock,
// which only a second goroutine sharing the lane id ever waits for. The one
// thing every thread shares on the report path is seq, an atomic counter that
// numbers each start: it orders the calls of different threads, which is all
// "a node shows its newest call" asks of a global order.
//
// A lane that ends joins its folds into the verdict each _Known holds and into
// calls, with atomic operations and no lock; a report takes mu never. So memory
// is the functions and the calls between them, however many calls the run
// makes. The failure to blame is the embedded _Blamer, which only a failed call
// takes.
//
// sites are the spawns the script writes, told when the run ends and read when
// the graph is drawn, to add the functions a stop kept the run from starting.
//
// Until the run ends the ledger draws an empty running graph: the folds are
// the lanes', and a graph read from them mid-run would be one no instant of
// the run had. A host that needs the graph as it goes asks for the live graph.
//
// Revisions:
//   - 2026-11-05 08:20: the verdicts of ended lanes live in the _Known, and the
//     calls in a sync.Map, so a lane retires with no lock (C53, Q67)
//   - 2026-11-10 00:25: renamed from _Ledger, as a test in tests/ needs it (C120a, Q314)
type Ledger struct {
	mu     sync.Mutex
	calls  sync.Map
	sites  []common.Spawn
	status workflowpb.Status
	ending *workflowpb.Cause
	_Blamer
	seq   atomic.Int64
	lanes sync.Map
	names sync.Map
}

// _Known is a function of the run, by identity: one per name, shared by
// every lane.
//
// verdict is what the lanes that ended say about the function: nil until one
// ends, and then the verdict with the highest start any of them held. A lane
// replaces it by compare-and-swap, so a retire takes no lock.
//
// Revisions:
//   - 2026-11-05 08:20: holds the settled verdict (C53, Q67)
type _Known struct {
	name    string
	verdict atomic.Pointer[_Verdict]
}

// _Pair is a call between two functions.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Pair struct {
	caller *_Known
	callee *_Known
}

// _Verdict is what a node shows for a function: the ordinal of the newest call
// started, and how that call ended, when it did.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Verdict struct {
	start  int64
	ended  bool
	status workflowpb.Status
}

// _Fold is one lane's account of one function: its newest start, how that call
// ended, and the functions this lane called from it. last is the callee last
// added to out, which spares a map write to a loop that calls one function
// again and again.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Fold struct {
	fn     *_Known
	start  int64
	ended  bool
	status workflowpb.Status
	out    map[*_Known]struct{}
	last   *_Known
}

// _Frame is a call a lane has started and not ended.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Frame struct {
	fold    *_Fold
	ordinal int64
}

// _Lane is one thread of the run. mu guards every other field.
//
// arrived orders the lanes still in a call at the end: the number seq gave the
// start, or the end, that last moved the lane to the function it executes. A
// number is taken by one move only, so no two lanes tie.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Lane struct {
	mu      sync.Mutex
	name    string
	frames  []_Frame
	folds   map[string]*_Fold
	arrived int64
	retired bool
}

// _Seat is a lane at a node when the graph is drawn.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Seat struct {
	arrived int64
	name    string
}

// NewLedger returns a ledger that has been told nothing.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-05 08:20: makes no map, since the verdicts and calls are lock-free (C53)
//   - 2026-11-10 00:25: renamed from _NewLedger, as a test in tests/ needs it (C120a, Q314)
func NewLedger() *Ledger {
	return new(Ledger)
}

// Started folds a call a thread began into the thread's own lane.
//
// A line that calls nothing folds nothing, as in the recorder. A later attempt
// of a repeat or a retry is the call already running, not another.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) Started(
	thread string,
	line *common.Line,
) {
	if line.Name == "" {
		return
	}

	if line.Child != "" {
		l._Spawned(
			thread,
			line,
		)

		return
	}

	lane := l._Enter(thread)
	defer lane.mu.Unlock()

	if line.Attempt > ONCE && lane._Advancing(line.Name) {
		return
	}

	caller := l._Push(
		lane,
		line.Name,
	)
	if caller != nil {
		caller._Call(lane.folds[line.Name].fn)
	}
}

// Ended folds how a call finished into the thread's lane, and blames a call
// that failed.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) Ended(
	thread string,
	name string,
	err error,
) {
	if name == "" {
		return
	}

	held, found := l.lanes.Load(thread)
	if !found {
		return
	}

	status, closed := l._Close(
		held.(*_Lane),
		name,
		err,
	)
	if closed {
		l._Blame(
			name,
			status,
			err,
		)
	}
}

// Began records that the run is going.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: renamed from _Began, as a test in tests/ needs it (C120a, Q314)
func (l *Ledger) Began() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.status = workflowpb.Status_STATUS_RUNNING
}

// Unstarted keeps the spawns the script writes, for _Folded to read against
// the functions the lanes hold. It adds nothing itself: the lanes may still be
// read, and a graph drawn is a function of what the lanes and the sites say.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-10 00:25: renamed from _Unstarted, as a test in tests/ needs it (C120a, Q314)
func (l *Ledger) Unstarted(sites []common.Spawn) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sites = sites
}

// Finished records how the run ended, and the cause when it failed. After it
// the ledger draws the graph.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: renamed from _Finished, as a test in tests/ needs it (C120a, Q314)
func (l *Ledger) Finished(
	status workflowpb.Status,
	cause *workflowpb.Cause,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.status = status
	l.ending = cause
}

// Drawn is the run's graph: an empty running graph until the run ends, and
// after it the graph a Recorder draws for the same reports.
//
// At the end every thread has reported its last call, so the lanes hold only
// threads a panic left inside a call; they are read one at a time, under their
// own locks, and listed at the node they are in.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-02 22:33: the fold after the running guard is _Folded (C27)
//   - 2026-11-10 00:25: renamed from _Drawn, as a test in tests/ needs it (C120a, Q314)
func (l *Ledger) Drawn() *workflowpb.Graph {
	l.mu.Lock()

	var (
		status = l.status
		ending = l.ending
	)

	l.mu.Unlock()

	if status == workflowpb.Status_STATUS_RUNNING {
		return &workflowpb.Graph{
			Status: status,
		}
	}

	return l._Folded(
		status,
		ending,
	)
}

// _Spawned folds a spawn: the call is the child lane's first, and the edge is
// from whatever the spawning lane is in.
//
// The two lanes are never held together, so two threads that spawn each other
// cannot wait for each other.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Spawned(
	thread string,
	line *common.Line,
) {
	held, found := l.lanes.Load(thread)
	if found {
		parent := held.(*_Lane)

		parent.mu.Lock()

		caller := parent._Innermost()
		if !parent.retired && caller != nil {
			caller._Call(l._Interned(line.Name))
		}

		parent.mu.Unlock()
	}

	child := l._Enter(line.Child)
	defer child.mu.Unlock()

	l._Push(
		child,
		line.Name,
	)
}

// _Push numbers a start and puts it on lane, which the caller holds. It
// returns the fold the lane was in, or nil.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Push(
	lane *_Lane,
	name string,
) *_Fold {
	fold := lane.folds[name]
	if fold == nil {
		fold = &_Fold{
			fn: l._Interned(name),
		}
		lane.folds[name] = fold
	}

	var (
		before  = lane._Innermost()
		ordinal = l.seq.Add(1)
	)

	fold.start = ordinal
	fold.ended = false
	lane.frames = append(
		lane.frames,
		_Frame{
			fold:    fold,
			ordinal: ordinal,
		},
	)

	if before == nil || before.fn != fold.fn {
		lane.arrived = ordinal
	}

	return before
}

// _Close ends the newest call to name on lane, and says whether there was one.
// A lane left in no call joins the ledger and is forgotten.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, Became, were _Ledger, _Became (C120a, Q314)
func (l *Ledger) _Close(
	lane *_Lane,
	name string,
	err error,
) (workflowpb.Status, bool) {
	lane.mu.Lock()
	defer lane.mu.Unlock()

	if lane.retired {
		return 0, false
	}

	fold := lane.folds[name]
	if fold == nil {
		return 0, false
	}

	at := lane._Newest(fold)
	if at == NONE {
		return 0, false
	}

	var (
		status    = Became(err)
		executing = lane._Innermost()
		ordinal   = lane.frames[at].ordinal
	)

	lane.frames = slices.Delete(
		lane.frames,
		at,
		at+1,
	)

	if fold.start == ordinal {
		fold.ended = true
		fold.status = status
	}

	now := lane._Innermost()
	if now != nil && now.fn != executing.fn {
		lane.arrived = l.seq.Add(1)
	}

	if len(lane.frames) == 0 {
		l._Retire(lane)
	}

	return status, true
}

// _Retire joins a lane that is in no call into the ledger and forgets it. The
// caller holds lane.mu; this takes no other lock.
//
// A later report on the same id finds the lane retired and makes a new one, so
// a thread that starts again after it ended folds into a lane of its own, and
// the join keeps the higher start number.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-05 08:20: takes no ledger lock (C53, Q67)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Retire(lane *_Lane) {
	for _, fold := range lane.folds {
		l._Settle(fold)
	}

	lane.retired = true
	lane.folds = nil

	l.lanes.CompareAndDelete(
		lane.name,
		lane,
	)
}

// _Settle joins one fold into the verdict of its function and into the calls.
// It takes no lock, and two lanes may settle one function at once.
//
// The verdict is replaced only by one with a higher start, and a lost
// compare-and-swap reads again, so the highest start stays whatever the order.
// A call is stored once however many lanes store it.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-05 08:20: swaps the verdict in the _Known and stores the calls in a
//     sync.Map, so no lock is held (C53, Q67)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Settle(fold *_Fold) {
	for {
		held := fold.fn.verdict.Load()
		if held != nil && held.start >= fold.start {
			break
		}

		if fold.fn.verdict.CompareAndSwap(
			held,
			&_Verdict{
				start:  fold.start,
				ended:  fold.ended,
				status: fold.status,
			},
		) {
			break
		}
	}

	for callee := range fold.out {
		l.calls.LoadOrStore(
			_Pair{
				caller: fold.fn,
				callee: callee,
			},
			struct{}{},
		)
	}
}

// _Join keeps in verdicts the newer of what it holds for fold's function and
// what fold says.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func _Join(
	verdicts map[*_Known]_Verdict,
	fold *_Fold,
) {
	held, known := verdicts[fold.fn]
	if known && held.start >= fold.start {
		return
	}

	verdicts[fold.fn] = _Verdict{
		start:  fold.start,
		ended:  fold.ended,
		status: fold.status,
	}
}

// _Enter is the lane of thread, made if the ledger has none, with its lock
// held.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Enter(thread string) *_Lane {
	for {
		held, found := l.lanes.Load(thread)
		if !found {
			held, _ = l.lanes.LoadOrStore(
				thread,
				&_Lane{
					name:  thread,
					folds: make(map[string]*_Fold),
				},
			)
		}

		lane := held.(*_Lane)

		lane.mu.Lock()

		if !lane.retired {
			return lane
		}

		lane.mu.Unlock()

		l.lanes.CompareAndDelete(
			thread,
			lane,
		)
	}
}

// _Interned is the one _Known named name.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Interned(name string) *_Known {
	held, found := l.names.Load(name)
	if found {
		return held.(*_Known)
	}

	held, _ = l.names.LoadOrStore(
		name,
		&_Known{
			name: name,
		},
	)

	return held.(*_Known)
}

// _Innermost is the fold of the newest call lane is in, or nil.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func (n *_Lane) _Innermost() *_Fold {
	if len(n.frames) == 0 {
		return nil
	}

	return n.frames[len(n.frames)-1].fold
}

// _Newest is the position of the newest call to fold on lane, or NONE.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func (n *_Lane) _Newest(fold *_Fold) int {
	for at := len(n.frames) - 1; at >= 0; at-- {
		if n.frames[at].fold == fold {
			return at
		}
	}

	return NONE
}

// _Advancing is true when lane is in a call to name, which a later attempt of a
// repeat or a retry advances rather than makes another.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func (n *_Lane) _Advancing(name string) bool {
	fold := n.folds[name]

	return fold != nil && n._Newest(fold) != NONE
}

// _Call records that this fold's function called callee, once however often.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func (f *_Fold) _Call(callee *_Known) {
	if f.last == callee {
		return
	}

	if f.out == nil {
		f.out = make(map[*_Known]struct{})
	}

	f.out[callee] = struct{}{}
	f.last = callee
}

// _Fault is nil: a ledger tells no watcher, so none can panic.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Fault() error {
	return nil
}

// _Standing is the graph as the lanes hold it now, the status RUNNING while
// the run goes: the same fold as Drawn, with no wait for the end.
//
// For a run that will not end by itself, which the memory watch gives up on.
// Each lane is read under its own lock and the lanes are never held together,
// so a thread keeps reporting while the graph is drawn, and the graph is not
// one instant of the run: a call a thread starts while the lanes are read may
// show on one lane and not on another, and an edge or a node can be missing.
// Every fold it shows was true at some moment, which is all a graph written as
// a run is abandoned claims.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:00: now uses Drawn, Ledger, were _Drawn, _Ledger (C120a, Q314)
func (l *Ledger) _Standing() *workflowpb.Graph {
	l.mu.Lock()

	var (
		status = l.status
		ending = l.ending
	)

	l.mu.Unlock()

	return l._Folded(
		status,
		ending,
	)
}

// _Folded joins the lanes and what the ended lanes settled into a graph of the
// given status.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation, lifted out of _Drawn for _Standing (C27)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Folded(
	status workflowpb.Status,
	ending *workflowpb.Cause,
) *workflowpb.Graph {
	var (
		verdicts = make(map[*_Known]_Verdict)
		calls    = make(map[_Pair]struct{})
		seats    = make(map[*_Known][]*_Seat)
	)

	l.lanes.Range(func(
		_,
		held any,
	) bool {
		held.(*_Lane)._Gather(
			verdicts,
			calls,
			seats,
		)

		return true
	})

	l._Settled(
		verdicts,
		calls,
	)
	l._Prevented(
		verdicts,
		calls,
	)

	graph := &workflowpb.Graph{
		Status: status,
		Functions: _Nodes(
			verdicts,
			seats,
		),
		Calls: _Edges(calls),
	}

	if ending != nil {
		graph.Cause = proto.CloneOf(ending)
	}

	return graph
}

// _Prevented adds a verdict NOT_STARTED and a call for each spawn the script
// writes in a function verdicts holds, of one it does not, by the rule the
// recorder keeps (_Prevented, the function).
//
// A graph drawn before the run ended has told no sites, and gains nothing.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Prevented(
	verdicts map[*_Known]_Verdict,
	calls map[_Pair]struct{},
) {
	l.mu.Lock()
	sites := l.sites
	l.mu.Unlock()

	prevented := _Prevented(
		sites,
		func(name string) bool {
			held, found := l.names.Load(name)
			if !found {
				return false
			}

			_, ran := verdicts[held.(*_Known)]

			return ran
		},
	)

	for _, site := range prevented {
		var (
			caller = l._Interned(site.Caller)
			target = l._Interned(site.Target)
		)

		verdicts[target] = _Verdict{
			ended:  true,
			status: workflowpb.Status_STATUS_NOT_STARTED,
		}
		calls[_Pair{
			caller: caller,
			callee: target,
		}] = struct{}{}
	}
}

// _Settled adds what the lanes that ended hold to verdicts and calls.
//
// It reads the verdict of every function the ledger knows and ranges over the
// calls, with no lock. _Folded reads the lanes first, and a lane settles before
// it leaves the lane map, so a lane that retires during the read shows here, or
// in the lane, or in both, and _Join absorbs both.
//
// Revisions:
//   - 2026-11-02 17:58: initial creation, lifted from _Drawn
//   - 2026-11-05 08:20: reads the verdicts in the names and the calls in the
//     sync.Map, with no lock (C53, Q67)
//   - 2026-11-10 01:00: now uses Ledger, were _Ledger (C120a, Q314)
func (l *Ledger) _Settled(
	verdicts map[*_Known]_Verdict,
	calls map[_Pair]struct{},
) {
	l.names.Range(func(
		_,
		held any,
	) bool {
		var (
			fn      = held.(*_Known)
			settled = fn.verdict.Load()
		)
		if settled == nil {
			return true
		}

		_Join(
			verdicts,
			&_Fold{
				fn:     fn,
				start:  settled.start,
				ended:  settled.ended,
				status: settled.status,
			},
		)

		return true
	})

	l.calls.Range(func(
		pair,
		_ any,
	) bool {
		calls[pair.(_Pair)] = struct{}{}

		return true
	})
}

// _Gather adds what this lane holds to verdicts and calls, and its seat to seats
// when it is inside a call.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func (n *_Lane) _Gather(
	verdicts map[*_Known]_Verdict,
	calls map[_Pair]struct{},
	seats map[*_Known][]*_Seat,
) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, fold := range n.folds {
		_Join(
			verdicts,
			fold,
		)

		for callee := range fold.out {
			calls[_Pair{
				caller: fold.fn,
				callee: callee,
			}] = struct{}{}
		}
	}

	inner := n._Innermost()
	if inner == nil {
		return
	}

	seats[inner.fn] = append(
		seats[inner.fn],
		&_Seat{
			arrived: n.arrived,
			name:    n.name,
		},
	)
}

// _Nodes is the node of each function, sorted by name. A node shows RUNNING
// until its newest call ended, and lists the lanes still inside it in the order
// they arrived.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func _Nodes(
	verdicts map[*_Known]_Verdict,
	seats map[*_Known][]*_Seat,
) []*workflowpb.Node {
	nodes := make(
		[]*workflowpb.Node,
		0,
		len(verdicts),
	)

	for fn, verdict := range verdicts {
		node := &workflowpb.Node{
			Name:   fn.name,
			Status: workflowpb.Status_STATUS_RUNNING,
		}

		if verdict.ended {
			node.Status = verdict.status
		}

		node.Threads = _Arrived(seats[fn])

		nodes = append(
			nodes,
			node,
		)
	}

	slices.SortFunc(
		nodes,
		func(
			a,
			b *workflowpb.Node,
		) int {
			return strings.Compare(
				a.Name,
				b.Name,
			)
		},
	)

	return nodes
}

// _Arrived is the names of seats, in the order the lanes arrived, a tie going
// to the lower name. It sorts seats in place, and is nil for none.
//
// Revisions:
//   - 2026-11-02 17:58: initial creation, lifted from _Nodes
func _Arrived(seats []*_Seat) []string {
	var names []string

	slices.SortFunc(
		seats,
		func(
			a *_Seat,
			b *_Seat,
		) int {
			return cmp.Or(
				cmp.Compare(
					a.arrived,
					b.arrived,
				),
				strings.Compare(
					a.name,
					b.name,
				),
			)
		},
	)

	for _, seat := range seats {
		names = append(
			names,
			seat.name,
		)
	}

	return names
}

// _Edges is each call as an edge, sorted by caller and then callee.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
func _Edges(calls map[_Pair]struct{}) []*workflowpb.Edge {
	edges := make(
		[]*workflowpb.Edge,
		0,
		len(calls),
	)

	for pair := range calls {
		edges = append(
			edges,
			&workflowpb.Edge{
				Caller: pair.caller.name,
				Callee: pair.callee.name,
			},
		)
	}

	slices.SortFunc(
		edges,
		func(
			a,
			b *workflowpb.Edge,
		) int {
			return _Order(
				a,
				b.Caller,
				b.Callee,
			)
		},
	)

	return edges
}
