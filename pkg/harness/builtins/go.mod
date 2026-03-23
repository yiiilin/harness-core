module github.com/yiiilin/harness-core/pkg/harness/builtins

go 1.24

require (
	github.com/yiiilin/harness-core v1.0.1
	github.com/yiiilin/harness-core/modules v0.1.0
)

require (
	github.com/creack/pty v1.1.24 // indirect
	github.com/google/uuid v1.6.0 // indirect
)

replace github.com/yiiilin/harness-core => ../../..

replace github.com/yiiilin/harness-core/modules => ../../../modules
