.PHONY: build test test-short vet lint fmt clean verify-counts

build:
	go build ./...

# H3-GAP-087 — this repo polices its own count prose: canonical counts
# (scripts/test-count.txt) → live suite parity (`go test ./... -list '^Test'`)
# → battery parity against the sibling shim checkout → stale-literal sweep with
# explicit historical exemptions. Exit 0 pass / 1 drift / 2 guard misconfigured.
verify-counts:
	sh scripts/check-test-count.sh

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
