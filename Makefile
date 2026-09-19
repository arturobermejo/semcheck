GOLANGCI_LINT_VERSION := v2.13.2

.PHONY: all build fmt lint plugin test tools

all: fmt lint test

fmt:
	golangci-lint fmt

lint:
	golangci-lint run ./...

test:
	go test -race ./...

build:
	go build -o semcheck ./cmd/semcheck

# Builds ./custom-gcl, a golangci-lint with semcheck compiled in, following
# .custom-gcl.yml. Run it with: ./custom-gcl run -c .golangci.semcheck.yml
plugin:
	golangci-lint custom

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
