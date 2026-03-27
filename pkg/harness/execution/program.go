package execution

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type ProgramInputBindingKind string
type VerificationScope string

const (
	ProgramInputBindingLiteral          ProgramInputBindingKind = "literal"
	ProgramInputBindingOutputRef        ProgramInputBindingKind = "output_ref"
	ProgramInputBindingAttachment       ProgramInputBindingKind = "attachment"
	ProgramInputBindingRuntimeHandleRef ProgramInputBindingKind = "runtime_handle_ref"

	VerificationScopeStep      VerificationScope = "step"
	VerificationScopeTarget    VerificationScope = "target"
	VerificationScopeAggregate VerificationScope = "aggregate"

	ProgramMetadataKeyID                 = "program_id"
	ProgramMetadataKeyGroupID            = "program_group_id"
	ProgramMetadataKeyParentStepID       = "program_parent_step_id"
	ProgramMetadataKeyDependsOn          = "program_depends_on"
	ProgramMetadataKeyNodeID             = "program_node_id"
	ProgramMetadataKeyMaxConcurrency     = "program_max_concurrency"
	ProgramMetadataKeyNodeMaxConcurrency = "program_node_max_concurrency"
)

type ConcurrencyPolicy struct {
	MaxConcurrency int `json:"max_concurrency,omitempty"`
}

type Program struct {
	ProgramID   string             `json:"program_id,omitempty"`
	EntryNodes  []string           `json:"entry_nodes,omitempty"`
	Nodes       []ProgramNode      `json:"nodes,omitempty"`
	Concurrency *ConcurrencyPolicy `json:"concurrency,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
}

type ProgramNode struct {
	NodeID      string                `json:"node_id"`
	Title       string                `json:"title,omitempty"`
	Action      action.Spec           `json:"action"`
	Verify      *verify.Spec          `json:"verify,omitempty"`
	VerifyScope VerificationScope     `json:"verify_scope,omitempty"`
	OnFail      *plan.OnFailSpec      `json:"on_fail,omitempty"`
	Targeting   *TargetSelection      `json:"targeting,omitempty"`
	Concurrency *ConcurrencyPolicy    `json:"concurrency,omitempty"`
	DependsOn   []string              `json:"depends_on,omitempty"`
	InputBinds  []ProgramInputBinding `json:"input_binds,omitempty"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
}

type ProgramInputBinding struct {
	Name          string                  `json:"name"`
	Kind          ProgramInputBindingKind `json:"kind"`
	Value         any                     `json:"value,omitempty"`
	Ref           *OutputRef              `json:"ref,omitempty"`
	Attachment    *AttachmentInput        `json:"attachment,omitempty"`
	RuntimeHandle *RuntimeHandleRef       `json:"runtime_handle,omitempty"`
	Metadata      map[string]any          `json:"metadata,omitempty"`
}

func (n ProgramNode) HasDependencies() bool {
	return len(n.DependsOn) > 0
}

func (n ProgramNode) HasInputBindings() bool {
	return len(n.InputBinds) > 0
}

func (n ProgramNode) MultiTargetRequested() bool {
	return n.Targeting != nil && n.Targeting.MultiTargetRequested()
}

func (b ProgramInputBinding) ReferencesOutput() bool {
	return b.Ref != nil
}

func (b ProgramInputBinding) HasAttachmentInput() bool {
	return b.Attachment != nil
}

func (b ProgramInputBinding) ReferencesRuntimeHandle() bool {
	return b.RuntimeHandle != nil
}
