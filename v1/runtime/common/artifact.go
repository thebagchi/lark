package common

// Artifact is a script and every module it loads, compiled together.
//
// Once built it owes nothing to the loader or the sources it came from: the
// code of every unit is in the artifact, in an order that initialises
// dependencies first. A host holds one, saves it with Save, and hands it to
// Start or Eval. Only lark compiles one: the artifact package inside the
// runtime holds the type that implements it (Q117), and a value of any other
// type is refused with runtime.ErrArtifact (Q118).
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: moved from artifact, as the interface of the Artifact
//     struct (C91w6, Q101, Q117)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 17:15: one-word names for its three sentinels, and Eval for Evaluate in the
//     text (C121, Q338, Q348)
type Artifact interface {
	// Save returns the artifact as one bundle, for the runtime's Load to read
	// back. It returns runtime.ErrUnit for an artifact with no entry unit, and
	// runtime.ErrMain for one whose entry point a run could not call.
	Save() ([]byte, error)
}

// CompileOption sets one thing on a compile.
//
// Options rather than parameters because a compile will grow knobs - a limit,
// a predeclared environment - and a parameter list cannot grow without breaking
// every caller. See CLAUDE.md, SOLID, open/closed.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: moved from artifact, as Option, and takes the
//     CompileSettings now in this package (C91w6, Q122)
type CompileOption func(settings *CompileSettings)

// CompileSettings is what options set on a compile. Plugins are where its
// script's names come from: the runtime's own, and whatever WithPlugins adds.
// Refused is why an option could not be taken, which an option cannot return
// and the compile does, before it reads the source: a second WithPlugins sets
// it (Q235). Where its modules come from is the source's own, not a setting.
//
// Exported fields, because an option written in another package sets them.
// Make one only through the artifact package's compile, and read it only
// there: it starts with the runtime's own plugins, which a struct literal
// leaves out.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: moved from artifact, as _Settings (C91w6, Q122)
//   - 2026-11-08 10:35: Refused carries a repeated option's refusal to the compile (C75, Q235)
type CompileSettings struct {
	Plugins []Plugin
	Refused error
}
