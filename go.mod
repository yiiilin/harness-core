module github.com/yiiilin/harness-core

go 1.24

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/lib/pq v1.10.9
	github.com/yiiilin/harness-core/adapters v0.0.0-20260324065650-154bedafd634
	github.com/yiiilin/harness-core/modules v0.0.0-20260324065650-154bedafd634
	github.com/yiiilin/harness-core/pkg/harness/builtins v0.0.0-20260324065650-154bedafd634
)

require github.com/creack/pty v1.1.24 // indirect

replace github.com/yiiilin/harness-core/adapters => ./adapters

replace github.com/yiiilin/harness-core/cmd/harness-core => ./cmd/harness-core

replace github.com/yiiilin/harness-core/modules => ./modules

replace github.com/yiiilin/harness-core/pkg/harness/builtins => ./pkg/harness/builtins
