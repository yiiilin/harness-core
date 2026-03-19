package permission

type Action string

const (
	Allow Action = "allow"
	Ask   Action = "ask"
	Deny  Action = "deny"
)

type Rule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     Action `json:"action"`
}

type Decision struct {
	Action      Action `json:"action"`
	Reason      string `json:"reason,omitempty"`
	MatchedRule string `json:"matched_rule,omitempty"`
}
