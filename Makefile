.PHONY: run build client

run:
	go run ./cmd/harness-core

build:
	go build ./cmd/harness-core

client:
	cd examples/go-client && go run .
