package action

type Spec struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args,omitempty"`
}

type Result struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
	Error *Error         `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
