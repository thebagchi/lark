# Third-party licences

Every library this repository depends on directly, and the licence it ships under. `CLAUDE.md`
asks that a licence be in the tree before the dependency is.

| Module | Licence | Used for |
| --- | --- | --- |
| [`go.starlark.net`](https://github.com/google/starlark-go) | BSD-3-Clause | the interpreter this runtime is built on |
| [`google.golang.org/protobuf`](https://github.com/protocolbuffers/protobuf-go) | BSD-3-Clause | the generated messages under `proto/gen/` |
| [`google.golang.org/grpc`](https://github.com/grpc/grpc-go) | Apache-2.0 | the `Host` service in `proto/plugin.proto`, so a plugin can live in another process |

The first two are BSD-3-Clause.
That licence asks that the copyright notice and the licence text stay with each redistribution
of their source or of a binary built from it.

gRPC is **Apache-2.0, which asks for more than that.** A redistribution must carry the
licence text and keep the attribution notices found in the source.
It must also state that files were changed, if any were.
If the upstream ships a `NOTICE` file, the redistribution must reproduce what that file says.
It also grants patent rights, and withdraws them from anyone who starts patent litigation over the work.

None of that binds anything today, because nothing here is redistributed.
All of it binds the first release.

gRPC was already in the module graph before this, at v1.83.1.
`buf` added it as a tool dependency.
On 2026-09-25, the own code of this repository started to import it.
Thus it is listed here and in the direct `require` block.
It is no longer only something that the toolchain happened to fetch.

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
worth an explicit statement.
A run of it over this source does not license this source.
The reason is that it is a separate program, and the build never links it.
Nothing here redistributes it. A release that
bundled the tool itself would be a different question, and no release does.
