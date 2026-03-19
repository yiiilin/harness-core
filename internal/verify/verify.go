package verify

type Check struct {
	Kind string         `json:"kind"`
	Args map[string]any `json:"args,omitempty"`
}

type Spec struct {
	Mode   string  `json:"mode"`
	Checks []Check `json:"checks"`
}
