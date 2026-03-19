package tool

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

var ErrToolNotRegistered = errors.New("tool not registered")

type Handler interface {
	Invoke(ctx context.Context, args map[string]any) (action.Result, error)
}

type Entry struct {
	Definition Definition
	Handler    Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Entry
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Entry{}}
}

func (r *Registry) Register(def Definition, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.ToolName] = Entry{Definition: def, Handler: handler}
}

func (r *Registry) Get(name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	return entry, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, v := range r.tools {
		out = append(out, v.Definition)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolName < out[j].ToolName })
	return out
}

func (r *Registry) Invoke(ctx context.Context, spec action.Spec) (action.Result, error) {
	entry, ok := r.Get(spec.ToolName)
	if !ok {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_REGISTERED", Message: spec.ToolName}}, ErrToolNotRegistered
	}
	if entry.Handler == nil {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_IMPLEMENTED", Message: spec.ToolName}}, nil
	}
	return entry.Handler.Invoke(ctx, spec.Args)
}
