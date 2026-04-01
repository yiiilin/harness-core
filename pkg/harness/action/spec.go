package action

type Spec struct {
	ToolName    string         `json:"tool_name"`
	ToolVersion string         `json:"tool_version,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
}

type Result struct {
	OK              bool           `json:"ok"`
	Data            map[string]any `json:"data,omitempty"`
	Meta            map[string]any `json:"meta,omitempty"`
	Error           *Error         `json:"error,omitempty"`
	Raw             *ResultPayload `json:"raw,omitempty"`
	WasTrimmed      bool           `json:"was_trimmed,omitempty"`
	RawSizeBytes    int            `json:"raw_size_bytes,omitempty"`
	InlineSizeChars int            `json:"inline_size_chars,omitempty"`
}

type ResultPayload struct {
	Data  map[string]any `json:"data,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
	Error *Error         `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
