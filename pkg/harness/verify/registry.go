package verify

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

var ErrVerifierNotRegistered = errors.New("verifier not registered")

type Definition struct {
	Kind        string         `json:"kind"`
	Description string         `json:"description,omitempty"`
	ArgsSchema  map[string]any `json:"args_schema,omitempty"`
}

type Checker interface {
	Verify(ctx context.Context, args map[string]any, result action.Result, state session.State) (Result, error)
}

type Entry struct {
	Definition Definition
	Checker    Checker
}

type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func NewRegistry() *Registry {
	return &Registry{entries: map[string]Entry{}}
}

func (r *Registry) Register(def Definition, checker Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[def.Kind] = Entry{Definition: def, Checker: checker}
}

func (r *Registry) Get(kind string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[kind]
	return entry, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.entries))
	for _, v := range r.entries {
		out = append(out, v.Definition)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func (r *Registry) Evaluate(ctx context.Context, spec Spec, result action.Result, state session.State) (Result, error) {
	if len(spec.Checks) == 0 {
		return Result{Success: true, Reason: "no checks configured"}, nil
	}

	mode := spec.Mode
	if mode == "" {
		mode = ModeAll
	}

	aggregated := map[string]any{}
	var anySuccess bool
	var allSuccess = true
	var firstError error
	var lastReason string

	for _, check := range spec.Checks {
		entry, ok := r.Get(check.Kind)
		if !ok {
			allSuccess = false
			lastReason = "verifier not registered: " + check.Kind
			if firstError == nil {
				firstError = ErrVerifierNotRegistered
			}
			continue
		}
		if entry.Checker == nil {
			allSuccess = false
			lastReason = "verifier has no checker: " + check.Kind
			continue
		}
		res, err := entry.Checker.Verify(ctx, check.Args, result, state)
		aggregated[check.Kind] = res
		if err != nil && firstError == nil {
			firstError = err
		}
		if res.Success {
			anySuccess = true
		} else {
			allSuccess = false
			if res.Reason != "" {
				lastReason = res.Reason
			}
		}
	}

	success := false
	switch mode {
	case ModeAny:
		success = anySuccess
	default:
		success = allSuccess
	}

	return Result{Success: success, Details: aggregated, Reason: lastReason}, firstError
}
