# Build entry points for this module. Every command a reader needs is a target
# here, so nothing has to be remembered from prose.

# Generated files are dropped: a formatter complaint about a file the generator
# owns is unactionable. .poc/go is a module of its own and is handled beside it.
GO_FILES := $(shell find . -name '*.go' -not -path './proto/gen/*' -not -path './.poc/*' -print)

# Everything a binary is built from. Wider than GO_FILES, which drops generated
# code because a formatter has nothing to say about a file it does not own - a
# build does. Two questions, two lists; sharing one would mean a change to a
# generated message never rebuilding the binary that carries it.
GO_SOURCES := $(shell find . -name '*.go' -not -path './.poc/*' -print) go.mod go.sum

.PHONY: all bootstrap generate tidy check lint lint-go lint-proto fmt build binaries vet test poc clean

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

lint-go:
	go tool golangci-lint run

lint-proto:
	go tool buf lint

# gofmt rather than go fmt, so generated files can be excluded: go fmt takes
# package patterns and would reformat files nobody may edit.
#
# The guard is not defensive clutter: GO_FILES is empty until the first
# hand-written file lands in this module, and gofmt with no arguments reads
# standard input and fails.
fmt:
	@if [ -n "$(GO_FILES)" ]; then gofmt -l -w $(GO_FILES); fi
	cd .poc/go && gofmt -l -w .

# Both modules: the planning POCs are a module of their own.
tidy:
	go mod tidy
	cd .poc/go && go mod tidy

build:
	go build ./...

# Every binary is a Makefile target and always carries an extension: .bin here,
# .exe on Windows. Adding a cmd/ means adding its target in the same change.
#
# Binaries land in bin/. tools/bin/ is for code generators, and there are none.
binaries: bin/lark.bin

# The prerequisites are the point. Without them make sees the file, calls it up
# to date and does nothing, so the binary is built once and never again - which
# is worse than having no target, because it ships stale code in silence.
bin/lark.bin: $(GO_SOURCES)
	@mkdir -p $(@D)
	go build -o $@ ./cmd/lark

vet:
	go vet ./...

test:
	go test ./... -race

# Planning POCs are a separate module and do not gate the suite, per
# .guidelines/working.md. They are still run, just not by test.
poc:
	cd .poc/go && go test ./... -race

clean:
	rm -rf proto/gen bin
