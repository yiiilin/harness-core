.PHONY: run build client test test-kernel test-builtins test-modules test-adapters test-cli test-workspace test-evals test-release release-check

run:
	go run ./cmd/harness-core

build:
	mkdir -p bin && go build -o ./bin/harness-core ./cmd/harness-core

client:
	cd examples/go-client && go run .

test:
	$(MAKE) test-workspace

test-kernel:
	go test ./... -count=1

test-builtins:
	cd pkg/harness/builtins && go test ./... -count=1

test-modules:
	cd modules && go test ./... -count=1

test-adapters:
	cd adapters && go test ./... -count=1

test-cli:
	cd cmd/harness-core && go test ./... -count=1

test-workspace: test-kernel test-builtins test-modules test-adapters test-cli

test-evals:
	go test ./evals -count=1

test-release:
	go test ./release -count=1

release-check: test-release test-evals
	@echo "release-check passed"
