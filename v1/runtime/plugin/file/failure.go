package file

import (
	"fmt"
)

// _Failed is a call in this module that did not work, carrying ErrFile
// without saying it.
//
// ErrFile answers "did a file call fail", which every refusal here has to
// answer yes to, so every refusal used to wrap it with %w. That put its words
// at the end of every message, where they read as a claim rather than as the
// category they are: a line of the wrong type and a read past the run's memory
// are not things the filesystem refused, and both said so. Carrying the
// sentinel through Unwrap instead keeps the question answerable and leaves the
// message to say only what happened.
type _Failed struct {
	cause error
}

// Error is what went wrong, and nothing about what kind of thing it was.
//
// Revisions:
//   - 2026-09-24 23:45: initial creation
func (f *_Failed) Error() string {
	return f.cause.Error()
}

// Unwrap is the cause and the module's sentinel, so errors.Is finds both.
//
// Revisions:
//   - 2026-09-24 23:45: initial creation
func (f *_Failed) Unwrap() []error {
	return []error{f.cause, ErrFile}
}

// _Refused is a failure the filesystem reported.
//
// The path is not named here because the error already carries it: os.Stat
// fails with "stat /x: no such file or directory", and naming it again would
// print it twice.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-09-24 23:45: carries ErrFile rather than wrapping its words in
func _Refused(who string, cause error) error {
	return &_Failed{cause: fmt.Errorf("%s: %w", who, cause)}
}

// _Rejected is a failure this module decided on rather than the filesystem.
//
// The path is named here, because a reason this module invented has nothing
// carrying it.
//
// Revisions:
//   - 2026-09-24 23:45: initial creation
func _Rejected(who string, named string, cause error) error {
	return &_Failed{cause: fmt.Errorf("%s: %s: %w", who, named, cause)}
}
