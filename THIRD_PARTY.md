# Third-party licences

Every library this repository depends on directly, and the licence it ships under. `CLAUDE.md`
asks that a licence be in the tree before the dependency is.

| Module | Licence | Used for |
| --- | --- | --- |
| [`go.starlark.net`](https://github.com/google/starlark-go) | BSD-3-Clause | the interpreter this runtime is built on |
| [`google.golang.org/protobuf`](https://github.com/protocolbuffers/protobuf-go) | BSD-3-Clause | the generated messages under `proto/gen/` |

Both are BSD-3-Clause, which asks that the copyright notice and the licence text be kept
with any redistribution of their source or of a binary built from it.

`github.com/google/uuid` was here until 2026-09-23, for run ids. Nothing mints one any more:
a host is handed the run itself, so there is nothing to name. It remains an indirect
dependency, which this table does not cover.

This repository redistributes none of them: `go.mod` names a module and whoever builds fetches
it themselves. **A released binary is different.** `bin/lark.bin` statically links them, so
a binary handed to anyone carries their code, and the notices go with it.

Nothing here builds a release yet. When something does, it carries these licence texts — and
whatever an indirect dependency's licence asks, which is a question for whoever releases.
