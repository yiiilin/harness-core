package verify

type Mode string

const (
	ModeAll Mode = "all"
	ModeAny Mode = "any"
)

type Check struct {
	Kind string         `json:"kind"`
	Args map[string]any `json:"args,omitempty"`
}

type Spec struct {
	Mode   Mode    `json:"mode"`
	Checks []Check `json:"checks"`
}
