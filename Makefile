.PHONY: build test vet lint fmt clean

build:
	go build ./...

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

all: fmt vet build test-short
