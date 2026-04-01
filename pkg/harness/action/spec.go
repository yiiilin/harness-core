package action

type Spec struct {
	ToolName    string         `json:"tool_name"`
	ToolVersion string         `json:"tool_version,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
}

type Result struct {
	OK        bool             `json:"ok"`
	Data      map[string]any   `json:"data,omitempty"`
	Meta      map[string]any   `json:"meta,omitempty"`
	Error     *Error           `json:"error,omitempty"`
	Raw       *ResultPayload   `json:"raw,omitempty"`
	Window    *ResultWindow    `json:"window,omitempty"`
	RawHandle *RawResultHandle `json:"raw_handle,omitempty"`
}

type ResultPayload struct {
	Data  map[string]any `json:"data,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
	Error *Error         `json:"error,omitempty"`
}

type ResultWindow struct {
	Truncated      bool  `json:"truncated,omitempty"`
	OriginalBytes  int   `json:"original_bytes,omitempty"`
	ReturnedBytes  int   `json:"returned_bytes,omitempty"`
	OriginalChars  int   `json:"original_chars,omitempty"`
	ReturnedChars  int   `json:"returned_chars,omitempty"`
	OriginalLines  int   `json:"original_lines,omitempty"`
	ReturnedLines  int   `json:"returned_lines,omitempty"`
	HasMore        bool  `json:"has_more,omitempty"`
	NextOffset     int64 `json:"next_offset,omitempty"`
	NextLineOffset int   `json:"next_line_offset,omitempty"`
}

type RawResultHandle struct {
	Kind   string `json:"kind,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Reread bool   `json:"reread,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
