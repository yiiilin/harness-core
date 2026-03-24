.PHONY: run build client test test-kernel test-builtins test-modules test-adapters test-cli test-workspace test-evals test-release test-external-consumers sync-companion-versions check-companion-versions release-check release-preflight release-resolve release-tag

REPO_LOCAL_VERIFY := bash $(CURDIR)/scripts/with_repo_local_module_env.sh

run:
	go run ./cmd/harness-core

build:
	mkdir -p bin && go build -o ./bin/harness-core ./cmd/harness-core

client:
	cd examples/go-client && go run .

test:
	$(MAKE) test-workspace

test-kernel:
	$(REPO_LOCAL_VERIFY) go test ./... -count=1

test-builtins:
	cd pkg/harness/builtins && $(REPO_LOCAL_VERIFY) go test ./... -count=1

test-modules:
	cd modules && $(REPO_LOCAL_VERIFY) go test ./... -count=1

test-adapters:
	cd adapters && $(REPO_LOCAL_VERIFY) go test ./... -count=1

test-cli:
	cd cmd/harness-core && $(REPO_LOCAL_VERIFY) go test ./... -count=1

test-workspace: test-kernel test-builtins test-modules test-adapters test-cli

test-evals:
	$(REPO_LOCAL_VERIFY) go test ./evals -count=1

test-release:
	$(REPO_LOCAL_VERIFY) go test ./release -count=1

test-external-consumers:
	$(REPO_LOCAL_VERIFY) go test ./release -run TestExternalConsumersBuildAgainstSnapshotRepo -count=1

sync-companion-versions:
	go run ./scripts/sync_companion_versions.go

check-companion-versions:
	go run ./scripts/sync_companion_versions.go --check

release-check: check-companion-versions test-release test-evals
	@echo "release-check passed"

release-preflight:
	$(MAKE) test-workspace
	$(MAKE) release-check

release-resolve:
	bash ./scripts/release-module.sh resolve $(MODULE) $(VERSION)

release-tag:
	bash ./scripts/release-module.sh tag $(MODULE) $(VERSION)
