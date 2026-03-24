package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestTargetSelectionMultiTargetRequested(t *testing.T) {
	cases := []struct {
		name string
		in   execution.TargetSelection
		want bool
	}{
		{
			name: "single target default",
			in: execution.TargetSelection{
				Targets: []execution.Target{{TargetID: "t1", Kind: "host"}},
			},
			want: false,
		},
		{
			name: "two explicit targets",
			in: execution.TargetSelection{
				Targets: []execution.Target{
					{TargetID: "t1", Kind: "host"},
					{TargetID: "t2", Kind: "host"},
				},
			},
			want: true,
		},
		{
			name: "fanout explicit mode",
			in: execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
			},
			want: true,
		},
		{
			name: "fanout all mode",
			in: execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutAll,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		if got := tc.in.MultiTargetRequested(); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestTargetSelectionMarshalOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(execution.TargetSelection{})
	if err != nil {
		t.Fatalf("marshal target selection: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("expected empty target selection to marshal as {}, got %s", data)
	}
}
