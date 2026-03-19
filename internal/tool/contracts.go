package tool

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Definition struct {
	ToolName       string    `json:"tool_name"`
	Version        string    `json:"version"`
	CapabilityType string    `json:"capability_type"`
	RiskLevel      RiskLevel `json:"risk_level"`
}
