# Third-party licences

Every library this repository depends on directly, and the licence it ships under. `CLAUDE.md`
asks that a licence be in the tree before the dependency is.

| Module | Licence | Used for |
| --- | --- | --- |
| [`go.starlark.net`](https://github.com/google/starlark-go) | BSD-3-Clause | the interpreter this runtime is built on |
| [`google.golang.org/protobuf`](https://github.com/protocolbuffers/protobuf-go) | BSD-3-Clause | the generated messages under `proto/gen/` |
| [`github.com/google/uuid`](https://github.com/google/uuid) | BSD-3-Clause | run ids |

All three are BSD-3-Clause, which asks that the copyright notice and the licence text be kept
with any redistribution of their source or of a binary built from it.

This repository redistributes none of them: `go.mod` names a module and whoever builds fetches
it themselves. **A released binary is different.** `bin/lark.bin` statically links all three, so
a binary handed to anyone carries their code, and the notices go with it.

Nothing here builds a release yet. When something does, it carries these three licence texts.
