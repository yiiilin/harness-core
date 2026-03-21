.PHONY: run build client test test-evals test-release release-check

run:
	go run ./cmd/harness-core

build:
	go build ./cmd/harness-core

client:
	cd examples/go-client && go run .

test:
	go test ./... -count=1

test-evals:
	go test ./evals -count=1

test-release:
	go test ./release -count=1

release-check: test-release test-evals
	@echo "release-check passed"
