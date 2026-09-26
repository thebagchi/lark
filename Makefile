# Build entry points for this module. Every command a reader needs is a target
# here, so nothing has to be remembered from prose.

# Generated files are dropped: a formatter complaint about a file the generator
# owns is unactionable.
GO_FILES := $(shell find . -name '*.go' -not -path './proto/gen/*' -print)

# Hand-written schemas. Generated output is Go under proto/gen/, so a find
# for .proto is already the files a formatter may touch.
PROTO_FILES := $(shell find proto -name '*.proto' -print)

# Everything a binary is built from. Wider than GO_FILES, which drops generated
# code because a formatter has nothing to say about a file it does not own - a
# build does. Two questions, two lists; sharing one would mean a change to a
# generated message never rebuilding the binary that carries it.
GO_SOURCES := $(shell find . -name '*.go' -print) go.mod go.sum

.PHONY: all bootstrap generate tidy check lint lint-go lint-proto fmt build binaries vet test poc clean

# all no longer runs poc: there is no .poc module. The practice stays in
# .guidelines/working.md - a new signature is still planned with one - and the
# next POC creates its module in one command.
all: generate build check test

# Everything that reads the code without changing it. This is the gate
# CLAUDE.md's before-done checklist asks for, minus the test run.
check: vet lint

# Bootstrapping is one command in Go: go.mod and go.sum already carry every pin.
bootstrap:
	go mod download

# Regenerates proto/gen from proto/. Never edit its output.
generate:
	go tool buf generate

lint: lint-go lint-proto

# --new-from-rev is CLAUDE.md's rule made mechanical: "New rules apply to new and
# edited code immediately; do not sweep existing code unless asked." The seven
# checkers go.md names were enabled on 2026-09-26 and found 130 findings in code
# written before they ran - 107 of them one formatting rule that had never been
# measured as written. Sweeping those is a cosmetic change to 41 files; letting
# them through unremarked is a gate nobody believes. So the gate holds what is
# being written now, and `make audit` is the backlog.
#
# HEAD~ rather than HEAD, so a change is still checked after it is committed
# rather than only while it is uncommitted.
lint-go:
	go tool golangci-lint run --new-from-rev=HEAD~

lint-proto:
	go tool buf lint

# gofmt rather than go fmt, so generated files can be excluded: go fmt takes
# package patterns and would reformat files nobody may edit.
#
# The guard is not defensive clutter: GO_FILES is empty until the first
# hand-written file lands in this module, and gofmt with no arguments reads
# standard input and fails. Proto files have the same empty-list guard:
# tools/fmt_proto.py with no arguments prints usage and exits 1.
fmt:
	@if [ -n "$(GO_FILES)" ]; then gofmt -l -w $(GO_FILES); fi
# tools/fmt_proto.py is the proto formatter, and buf format is not. The two
# disagree: this one aligns the = of a field with its neighbours, buf format
# takes that alignment out. Running buf format would rewrite every .proto here -
# measured 2026-09-25 at 775 diff lines - so it is not a step, not in a gate,
# and not a tidy-up to reach for. buf lint and buf generate are unaffected.
	@if [ -n "$(PROTO_FILES)" ]; then python3 tools/fmt_proto.py $(PROTO_FILES); fi

# Both modules: the planning POCs are a module of their own.
tidy:
	go mod tidy

build:
	go build ./...

# Every binary is a Makefile target and always carries an extension: .bin here,
# .exe on Windows. Adding a cmd/ means adding its target in the same change.
#
# Binaries land in bin/. tools/bin/ is for code generators, and there are none.
binaries: bin/lark.bin bin/lark-clock.bin

# The prerequisites are the point. Without them make sees the file, calls it up
# to date and does nothing, so the binary is built once and never again - which
# is worse than having no target, because it ships stale code in silence.
bin/lark.bin: $(GO_SOURCES)
	@mkdir -p $(@D)
	go build -o $@ ./cmd/lark

# clock is a plugin that lives in its own process, and the worked example of
# how to write one. It is built because an example nobody compiles is an
# example that stops being true.
#
# lark- because that is the prefix a host searches a plugin directory for, the
# way terraform-provider-aws is named. A plugin whose name does not match is a
# plugin nothing finds.
bin/lark-clock.bin: $(GO_SOURCES)
	@mkdir -p $(@D)
	go build -o $@ ./cmd/clock

# Every finding in the whole tree, with the output caps off. The caps are why
# this was mismeasured for a week: golangci-lint stops at 50 per linter by
# default, so a survey that reported 50 was reporting a ceiling. Not a gate -
# the count is recorded in .doc/todo.md and cleared when somebody asks.
audit:
	go tool golangci-lint run --max-issues-per-linter=0 --max-same-issues=0

vet:
	go vet ./...

test:
	go test ./... -race

clean:
	rm -rf proto/gen bin
