package tool

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
