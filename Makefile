GOLANGCI_LINT_VERSION := v2.13.2

.PHONY: all build fmt lint test tools

all: fmt lint test

fmt:
	golangci-lint fmt

lint:
	golangci-lint run ./...

test:
	go test -race ./...

build:
	go build -o semcheck ./cmd/semcheck

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
