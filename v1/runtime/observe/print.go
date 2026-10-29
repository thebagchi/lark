package observe

// Print is one line a run's script printed: Thread, the id of the thread that
// printed it, Function, the function that called print, and Text, the line.
//
// The same three things the run's logger is told as a record, so a host that
// reads them here and one that reads its log see one line the same way.
type Print struct {
	Thread   string
	Function string
	Text     string
}
