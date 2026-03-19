package harness

import hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"

// Options is the main runtime construction options struct.
type Options = hruntime.Options

// Service is the main harness runtime service.
type Service = hruntime.Service

// New constructs a runtime service with defaults applied.
func New(opts Options) *Service {
	return hruntime.New(opts)
}

// RegisterBuiltins wires the default built-in tools and verifiers into options.
func RegisterBuiltins(opts *Options) {
	hruntime.RegisterBuiltins(opts)
}
