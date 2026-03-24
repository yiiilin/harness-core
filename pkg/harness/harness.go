package harness

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/replay"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
	"github.com/yiiilin/harness-core/pkg/harness/worker"
)

// Core constructor and options.
type Options = hruntime.Options
type Service = hruntime.Service

// Common domain types.
type TaskSpec = task.Spec
type TaskRecord = task.Record
type TaskStatus = task.Status

type SessionState = session.State
type SessionPhase = session.Phase

type PlanSpec = plan.Spec
type PlanStatus = plan.Status

type StepSpec = plan.StepSpec
type StepStatus = plan.StepStatus

type PlanningRecord = planning.Record
type PlanningStatus = planning.Status

type ActionSpec = action.Spec
type ActionResult = action.Result

type ApprovalRequest = approval.Request
type ApprovalResponse = approval.Response
type ApprovalRecord = approval.Record
type ApprovalReply = approval.Reply
type ApprovalStatus = approval.Status

type CapabilitySnapshot = capability.Snapshot
type CapabilityResolution = capability.Resolution
type CapabilityMatchResult = capability.MatchResult
type CapabilityUnsupportedReason = capability.UnsupportedReason
type CapabilityUnsupportedReasonCode = capability.UnsupportedReasonCode
type CapabilitySupportRequirements = capability.SupportRequirements

type ExecutionAttempt = execution.Attempt
type ExecutionAttemptStatus = execution.AttemptStatus
type ExecutionActionRecord = execution.ActionRecord
type ExecutionActionStatus = execution.ActionStatus
type ExecutionVerificationRecord = execution.VerificationRecord
type ExecutionVerificationStatus = execution.VerificationStatus
type ExecutionArtifact = execution.Artifact
type ExecutionRuntimeHandle = execution.RuntimeHandle
type ExecutionInteractiveCapabilities = execution.InteractiveCapabilities
type ExecutionInteractiveSnapshot = execution.InteractiveSnapshot
type ExecutionInteractiveObservation = execution.InteractiveObservation
type ExecutionInteractiveOperation = execution.InteractiveOperation
type ExecutionInteractiveOperationKind = execution.InteractiveOperationKind
type ExecutionInteractiveRuntime = execution.InteractiveRuntime
type ExecutionCycle = execution.ExecutionCycle
type ExecutionBlockedRuntime = execution.BlockedRuntime
type ExecutionBlockedRuntimeKind = execution.BlockedRuntimeKind
type ExecutionBlockedRuntimeStatus = execution.BlockedRuntimeStatus
type ExecutionTarget = execution.Target
type ExecutionTargetRef = execution.TargetRef
type ExecutionTargetSelection = execution.TargetSelection
type ExecutionTargetSelectionMode = execution.TargetSelectionMode
type ExecutionTargetFailureStrategy = execution.TargetFailureStrategy
type ExecutionAggregateScope = execution.AggregateScope
type ExecutionAggregateStatus = execution.AggregateStatus
type ExecutionAggregateTargetResult = execution.AggregateTargetResult
type ExecutionAggregateResult = execution.AggregateResult
type ExecutionAttachmentInput = execution.AttachmentInput
type ExecutionAttachmentInputKind = execution.AttachmentInputKind
type ExecutionAttachmentMaterialization = execution.AttachmentMaterialization
type ExecutionArtifactRef = execution.ArtifactRef
type ExecutionAttachmentRef = execution.AttachmentRef
type ExecutionOutputRef = execution.OutputRef
type ExecutionOutputRefKind = execution.OutputRefKind
type ExecutionProgram = execution.Program
type ExecutionProgramNode = execution.ProgramNode
type ExecutionProgramInputBinding = execution.ProgramInputBinding
type ExecutionProgramInputBindingKind = execution.ProgramInputBindingKind
type ExecutionVerificationScope = execution.VerificationScope
type ExecutionTargetSlice = execution.TargetSlice
type ExecutionBlockedRuntimeProjection = execution.BlockedRuntimeProjection
type ExecutionBlockedRuntimeWait = execution.BlockedRuntimeWait
type ExecutionBlockedRuntimeWaitScope = execution.BlockedRuntimeWaitScope
type ExecutionBlockedRuntimeRecord = execution.BlockedRuntimeRecord
type ExecutionBlockedRuntimeSubject = execution.BlockedRuntimeSubject
type ExecutionBlockedRuntimeCondition = execution.BlockedRuntimeCondition
type ExecutionBlockedRuntimeConditionKind = execution.BlockedRuntimeConditionKind

type ReplaySessionReader = replay.SessionReader
type ReplayCycleReader = replay.CycleReader
type ReplayReader = replay.ExecutionFactReader
type ReplayProjectionReader = replay.Reader
type ReplaySessionProjection = replay.SessionProjection
type ReplayExecutionCycleProjection = replay.ExecutionCycleProjection

type WorkerRuntime = worker.Runtime
type WorkerOptions = worker.Options
type WorkerLoopOptions = worker.LoopOptions
type WorkerLoopIteration = worker.LoopIteration
type WorkerResult = worker.Result
type WorkerHelper = worker.Worker

type VerifySpec = verify.Spec
type VerifyResult = verify.Result

type ToolDefinition = tool.Definition
type ToolRiskLevel = tool.RiskLevel

type AuditEvent = audit.Event

type PermissionDecision = permission.Decision
type PermissionAction = permission.Action

type ContextAssembler = hruntime.ContextAssembler
type ContextPackage = hruntime.ContextPackage
type ContextSummary = hruntime.ContextSummary
type StepRunOutput = hruntime.StepRunOutput
type SessionRunOutput = hruntime.SessionRunOutput
type AbortRequest = hruntime.AbortRequest
type AbortOutput = hruntime.AbortOutput
type InteractiveRuntimeUpdate = hruntime.InteractiveRuntimeUpdate
type RuntimeHandleUpdate = hruntime.RuntimeHandleUpdate
type RuntimeHandleCloseRequest = hruntime.RuntimeHandleCloseRequest
type RuntimeHandleInvalidateRequest = hruntime.RuntimeHandleInvalidateRequest
type CompactionTrigger = hruntime.CompactionTrigger
type CompactionPolicy = hruntime.CompactionPolicy
type LoopBudgets = hruntime.LoopBudgets
type Planner = hruntime.Planner
type EventSink = hruntime.EventSink
type MetricsExporter = hruntime.MetricsExporter
type TraceExporter = hruntime.TraceExporter

type PolicyEvaluator = permission.Evaluator

const (
	RiskLow    = tool.RiskLow
	RiskMedium = tool.RiskMedium
	RiskHigh   = tool.RiskHigh

	Allow = permission.Allow
	Ask   = permission.Ask
	Deny  = permission.Deny

	CompactionTriggerPlan    = hruntime.CompactionTriggerPlan
	CompactionTriggerExecute = hruntime.CompactionTriggerExecute
	CompactionTriggerRecover = hruntime.CompactionTriggerRecover

	CapabilityReasonNotFound            = capability.ReasonCapabilityNotFound
	CapabilityReasonDisabled            = capability.ReasonCapabilityDisabled
	CapabilityReasonVersionNotFound     = capability.ReasonCapabilityVersionNotFound
	CapabilityReasonViewNotFound        = capability.ReasonCapabilityViewNotFound
	CapabilityReasonViewDrift           = capability.ReasonCapabilityViewDrift
	CapabilityReasonMultiTargetFanout   = capability.ReasonMultiTargetFanoutUnsupported
	CapabilityReasonPreplannedToolGraph = capability.ReasonPreplannedToolGraphUnsupported
	CapabilityReasonInteractiveReopen   = capability.ReasonInteractiveReopenUnsupported
	CapabilityReasonArtifactInput       = capability.ReasonArtifactInputUnsupported

	ExecutionBlockedRuntimeApproval     = execution.BlockedRuntimeApproval
	ExecutionBlockedRuntimeConfirmation = execution.BlockedRuntimeConfirmation
	ExecutionBlockedRuntimeExternal     = execution.BlockedRuntimeExternal
	ExecutionBlockedRuntimeInteractive  = execution.BlockedRuntimeInteractive
	ExecutionBlockedRuntimePending      = execution.BlockedRuntimePending
	ExecutionBlockedRuntimeApproved     = execution.BlockedRuntimeApproved
	ExecutionBlockedRuntimeRejected     = execution.BlockedRuntimeRejected
	ExecutionBlockedRuntimeConfirmed    = execution.BlockedRuntimeConfirmed
	ExecutionBlockedRuntimeResumed      = execution.BlockedRuntimeResumed
	ExecutionBlockedRuntimeAborted      = execution.BlockedRuntimeAborted

	ExecutionTargetSelectionSingle         = execution.TargetSelectionSingle
	ExecutionTargetSelectionFanoutExplicit = execution.TargetSelectionFanoutExplicit
	ExecutionTargetSelectionFanoutAll      = execution.TargetSelectionFanoutAll
	ExecutionTargetFailureAbort            = execution.TargetFailureAbort
	ExecutionTargetFailureContinue         = execution.TargetFailureContinue
	ExecutionAggregateScopeTargetFanout    = execution.AggregateScopeTargetFanout
	ExecutionAggregateStatusPending        = execution.AggregateStatusPending
	ExecutionAggregateStatusCompleted      = execution.AggregateStatusCompleted
	ExecutionAggregateStatusPartialFailed  = execution.AggregateStatusPartialFailed
	ExecutionAggregateStatusFailed         = execution.AggregateStatusFailed
	ExecutionTargetArgKey                  = execution.TargetArgKey
	ExecutionTargetMetadataKeyID           = execution.TargetMetadataKeyID
	ExecutionTargetMetadataKeyKind         = execution.TargetMetadataKeyKind
	ExecutionTargetMetadataKeyName         = execution.TargetMetadataKeyName
	ExecutionTargetMetadataKeyIndex        = execution.TargetMetadataKeyIndex
	ExecutionTargetMetadataKeyCount        = execution.TargetMetadataKeyCount
	ExecutionAggregateMetadataKeyID        = execution.AggregateMetadataKeyID
	ExecutionAggregateMetadataKeyScope     = execution.AggregateMetadataKeyScope
	ExecutionAggregateMetadataKeyStrategy  = execution.AggregateMetadataKeyStrategy
	ExecutionAggregateMetadataKeyExpected  = execution.AggregateMetadataKeyExpected
	ExecutionAggregateMetadataKeyTitle     = execution.AggregateMetadataKeyTitle

	ExecutionInteractiveMetadataKeyEnabled             = execution.InteractiveMetadataKeyEnabled
	ExecutionInteractiveMetadataKeySupportsReopen      = execution.InteractiveMetadataKeySupportsReopen
	ExecutionInteractiveMetadataKeySupportsView        = execution.InteractiveMetadataKeySupportsView
	ExecutionInteractiveMetadataKeySupportsWrite       = execution.InteractiveMetadataKeySupportsWrite
	ExecutionInteractiveMetadataKeySupportsClose       = execution.InteractiveMetadataKeySupportsClose
	ExecutionInteractiveMetadataKeyNextOffset          = execution.InteractiveMetadataKeyNextOffset
	ExecutionInteractiveMetadataKeyClosed              = execution.InteractiveMetadataKeyClosed
	ExecutionInteractiveMetadataKeyExitCode            = execution.InteractiveMetadataKeyExitCode
	ExecutionInteractiveMetadataKeyStatus              = execution.InteractiveMetadataKeyStatus
	ExecutionInteractiveMetadataKeyStatusReason        = execution.InteractiveMetadataKeyStatusReason
	ExecutionInteractiveMetadataKeySnapshotArtifactID  = execution.InteractiveMetadataKeySnapshotArtifactID
	ExecutionInteractiveMetadataKeyLastOperationKind   = execution.InteractiveMetadataKeyLastOperationKind
	ExecutionInteractiveMetadataKeyLastOperationAt     = execution.InteractiveMetadataKeyLastOperationAt
	ExecutionInteractiveMetadataKeyLastOperationOffset = execution.InteractiveMetadataKeyLastOperationOffset
	ExecutionInteractiveMetadataKeyLastOperationBytes  = execution.InteractiveMetadataKeyLastOperationBytes

	ExecutionInteractiveOperationReopen = execution.InteractiveOperationReopen
	ExecutionInteractiveOperationView   = execution.InteractiveOperationView
	ExecutionInteractiveOperationWrite  = execution.InteractiveOperationWrite
	ExecutionInteractiveOperationClose  = execution.InteractiveOperationClose

	ExecutionAttachmentInputText           = execution.AttachmentInputText
	ExecutionAttachmentInputBytes          = execution.AttachmentInputBytes
	ExecutionAttachmentInputArtifactRef    = execution.AttachmentInputArtifactRef
	ExecutionAttachmentMaterializeNone     = execution.AttachmentMaterializeNone
	ExecutionAttachmentMaterializeTempFile = execution.AttachmentMaterializeTempFile

	ExecutionOutputRefStructured = execution.OutputRefStructured
	ExecutionOutputRefText       = execution.OutputRefText
	ExecutionOutputRefBytes      = execution.OutputRefBytes
	ExecutionOutputRefArtifact   = execution.OutputRefArtifact
	ExecutionOutputRefAttachment = execution.OutputRefAttachment

	ExecutionProgramInputBindingLiteral    = execution.ProgramInputBindingLiteral
	ExecutionProgramInputBindingOutputRef  = execution.ProgramInputBindingOutputRef
	ExecutionProgramInputBindingAttachment = execution.ProgramInputBindingAttachment
	ExecutionVerificationScopeStep         = execution.VerificationScopeStep
	ExecutionVerificationScopeTarget       = execution.VerificationScopeTarget
	ExecutionVerificationScopeAggregate    = execution.VerificationScopeAggregate

	ExecutionBlockedRuntimeWaitStep   = execution.BlockedRuntimeWaitStep
	ExecutionBlockedRuntimeWaitAction = execution.BlockedRuntimeWaitAction
	ExecutionBlockedRuntimeWaitTarget = execution.BlockedRuntimeWaitTarget

	ExecutionBlockedRuntimeConditionApproval     = execution.BlockedRuntimeConditionApproval
	ExecutionBlockedRuntimeConditionConfirmation = execution.BlockedRuntimeConditionConfirmation
	ExecutionBlockedRuntimeConditionExternal     = execution.BlockedRuntimeConditionExternal
	ExecutionBlockedRuntimeConditionInteractive  = execution.BlockedRuntimeConditionInteractive
)

// New constructs a runtime service with defaults applied.
func New(opts Options) *Service {
	return hruntime.New(opts)
}

// NewDefault constructs a runtime using default in-memory components.
func NewDefault() *Service {
	return hruntime.New(Options{})
}

// NewWorkerHelper constructs the public claim/renew/run/recover/release helper.
func NewWorkerHelper(opts WorkerOptions) (*WorkerHelper, error) {
	return worker.New(opts)
}

// NewReplayReader constructs the public replay/debug projection helper.
func NewReplayReader(source ReplaySessionReader) *ReplayProjectionReader {
	return replay.NewReader(source)
}
