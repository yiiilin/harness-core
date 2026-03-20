package tool

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

var ErrToolNotRegistered = errors.New("tool not registered")
var ErrToolDisabled = errors.New("tool disabled")

type Handler interface {
	Invoke(ctx context.Context, args map[string]any) (action.Result, error)
}

type Entry struct {
	Definition Definition
	Handler    Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]map[string]Entry
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]map[string]Entry{}}
}

func (r *Registry) Register(def Definition, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tools[def.ToolName] == nil {
		r.tools[def.ToolName] = map[string]Entry{}
	}
	r.tools[def.ToolName][versionKey(def.Version)] = Entry{Definition: def, Handler: handler}
}

func (r *Registry) Get(name string) (Entry, bool) {
	return r.Resolve(name, "")
}

func (r *Registry) GetVersion(name, version string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.tools[name]
	if !ok {
		return Entry{}, false
	}
	entry, ok := versions[versionKey(version)]
	return entry, ok
}

func (r *Registry) Resolve(name, version string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.tools[name]
	if !ok || len(versions) == 0 {
		return Entry{}, false
	}
	if strings.TrimSpace(version) != "" {
		entry, ok := versions[versionKey(version)]
		return entry, ok
	}
	keys := make([]string, 0, len(versions))
	for key := range versions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareVersion(keys[i], keys[j]) < 0
	})
	entry, ok := versions[keys[len(keys)-1]]
	return entry, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Definition{}
	for _, versions := range r.tools {
		for _, v := range versions {
			out = append(out, v.Definition)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ToolName == out[j].ToolName {
			return compareVersion(out[i].Version, out[j].Version) < 0
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out
}

func (r *Registry) Invoke(ctx context.Context, spec action.Spec) (action.Result, error) {
	entry, ok := r.Resolve(spec.ToolName, spec.ToolVersion)
	if !ok {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_REGISTERED", Message: spec.ToolName}}, ErrToolNotRegistered
	}
	if !entry.Definition.Enabled {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_DISABLED", Message: spec.ToolName}}, ErrToolDisabled
	}
	if entry.Handler == nil {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_IMPLEMENTED", Message: spec.ToolName}}, nil
	}
	return entry.Handler.Invoke(ctx, spec.Args)
}

func versionKey(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return "__default__"
	}
	return trimmed
}

func compareVersion(left, right string) int {
	li, lok := parseNumericVersion(left)
	ri, rok := parseNumericVersion(right)
	switch {
	case lok && rok:
		switch {
		case li < ri:
			return -1
		case li > ri:
			return 1
		default:
			return 0
		}
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func parseNumericVersion(version string) (int, bool) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" || trimmed == "__default__" {
		return 0, false
	}
	if len(trimmed) > 1 && (trimmed[0] == 'v' || trimmed[0] == 'V') {
		trimmed = trimmed[1:]
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return value, true
}
