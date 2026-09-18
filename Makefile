.PHONY: build test test-short vet lint fmt clean verify-counts release-drift

build:
	go build ./...

# H3-GAP-087 — this repo polices its own count prose: canonical counts
# (scripts/test-count.txt) → live suite parity (`go test ./... -list '^Test'`)
# → battery parity against the sibling shim checkout → stale-literal sweep with
# explicit historical exemptions. Exit 0 pass / 1 drift / 2 guard misconfigured.
verify-counts:
	sh scripts/check-test-count.sh

# GAP-038 — informational release-drift reporter: how far main has moved past
# the published tag, and how much of that is wire-facing (non-bookkeeping).
# Exits 0 by default; pass an explicit threshold through the script (e.g.
# `sh scripts/check-release-drift.sh --fail-over 20`) when a hard gate is
# wanted. Deliberately NOT part of `all`: a busy main is not a build failure.
release-drift:
	sh scripts/check-release-drift.sh

test:
	go test ./... -count=1

test-short:
	go test ./... -count=1 -short

vet:
	go vet ./...

lint:
	golangci-lint run ./... 2>/dev/null || staticcheck ./... 2>/dev/null || echo "lint: no linter available (install golangci-lint or staticcheck)"

fmt:
	gofmt -w .

clean:
	go clean ./...

all: verify-counts fmt vet build test-short
