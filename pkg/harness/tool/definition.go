package tool

import "sync"

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Definition struct {
	ToolName       string         `json:"tool_name"`
	Version        string         `json:"version"`
	CapabilityType string         `json:"capability_type"`
	InputSchema    map[string]any `json:"input_schema,omitempty"`
	ResultSchema   map[string]any `json:"result_schema,omitempty"`
	VerifySchema   map[string]any `json:"verify_schema,omitempty"`
	RiskLevel      RiskLevel      `json:"risk_level"`
	Enabled        bool           `json:"enabled"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Definition{}}
}

func (r *Registry) Register(def Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.ToolName] = def
}

func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[name]
	return def, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, v := range r.tools {
		out = append(out, v)
	}
	return out
}
