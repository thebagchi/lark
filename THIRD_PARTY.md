# Third-party licences

Every library this repository depends on directly, and the licence it ships under. `CLAUDE.md`
asks that a licence be in the tree before the dependency is.

| Module | Licence | Used for |
| --- | --- | --- |
| [`go.starlark.net`](https://github.com/google/starlark-go) | BSD-3-Clause | the interpreter this runtime is built on |
| [`google.golang.org/protobuf`](https://github.com/protocolbuffers/protobuf-go) | BSD-3-Clause | the generated messages under `proto/gen/` |
| [`google.golang.org/grpc`](https://github.com/grpc/grpc-go) | Apache-2.0 | the `Host` service in `proto/plugin.proto`, so a plugin can live in another process |

The first two are BSD-3-Clause, which asks that the copyright notice and the licence text be
kept with any redistribution of their source or of a binary built from it.

gRPC is **Apache-2.0, which asks for more than that.** A redistribution has to carry the
licence text, state that files were changed if any were, keep the attribution notices found in
the source, and — if the upstream ships a `NOTICE` file — reproduce what that file says. It also
grants patent rights, and withdraws them from anyone who starts patent litigation over the work.
None of that binds anything today, because nothing here is redistributed; all of it binds the
first release.

gRPC was already in the module graph before this, at v1.83.1, pulled in by `buf` as a tool
dependency. What changed on 2026-09-25 is that this repository's own code imports it, so it is
listed here and in the direct `require` block rather than being something the toolchain happened
to fetch.

`github.com/google/uuid` was here until 2026-09-23, for run ids. Nothing mints one any more:
a host is handed the run itself, so there is nothing to name. It remains an indirect
dependency, which this table does not cover.

This repository redistributes none of them: `go.mod` names a module and whoever builds fetches
it themselves. **A released binary is different.** `bin/lark.bin` statically links them, so
a binary handed to anyone carries their code, and the notices go with it.

Nothing here builds a release yet. When something does, it carries these licence texts — and
whatever an indirect dependency's licence asks, which is a question for whoever releases.

## Tools, which link into nothing

Pinned with `go get -tool` and run as commands. They are not imported, so nothing they bring
becomes part of `bin/lark.bin` or `bin/lark-clock.bin`.

| Tool | Licence | What it does here |
| --- | --- | --- |
| [`golangci-lint`](https://github.com/golangci/golangci-lint) | **GPL-3.0** | `make lint-go` |
| [`buf`](https://github.com/bufbuild/buf) | Apache-2.0 | `make generate`, `make lint-proto` |
| [`protoc-gen-go`](https://github.com/protocolbuffers/protobuf-go) | BSD-3-Clause | the messages in `proto/gen/` |
| [`protoc-gen-go-grpc`](https://github.com/grpc/grpc-go) | Apache-2.0 | the client and server for `proto/plugin.proto` |

**`golangci-lint` is GPL-3.0, and that is named here deliberately.** It is reciprocal, so it is
worth being explicit: running it over this source does not license this source, because it is
executed as a separate program and never linked in. Nothing here redistributes it. A release that
bundled the tool itself would be a different question, and no release does.
