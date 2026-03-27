package execution

const (
	InteractiveMetadataKeyEnabled             = "interactive"
	InteractiveMetadataKeySupportsReopen      = "interactive_supports_reopen"
	InteractiveMetadataKeySupportsView        = "interactive_supports_view"
	InteractiveMetadataKeySupportsWrite       = "interactive_supports_write"
	InteractiveMetadataKeySupportsClose       = "interactive_supports_close"
	InteractiveMetadataKeyNextOffset          = "interactive_next_offset"
	InteractiveMetadataKeyClosed              = "interactive_closed"
	InteractiveMetadataKeyExitCode            = "interactive_exit_code"
	InteractiveMetadataKeyStatus              = "interactive_status"
	InteractiveMetadataKeyStatusReason        = "interactive_status_reason"
	InteractiveMetadataKeySnapshotArtifactID  = "interactive_snapshot_artifact_id"
	InteractiveMetadataKeyLastOperationKind   = "interactive_last_operation_kind"
	InteractiveMetadataKeyLastOperationAt     = "interactive_last_operation_at"
	InteractiveMetadataKeyLastOperationOffset = "interactive_last_operation_offset"
	InteractiveMetadataKeyLastOperationBytes  = "interactive_last_operation_bytes"
)

type InteractiveOperationKind string

const (
	InteractiveOperationReopen InteractiveOperationKind = "reopen"
	InteractiveOperationView   InteractiveOperationKind = "view"
	InteractiveOperationWrite  InteractiveOperationKind = "write"
	InteractiveOperationClose  InteractiveOperationKind = "close"
)

type InteractiveCapabilities struct {
	Reopen bool `json:"reopen,omitempty"`
	View   bool `json:"view,omitempty"`
	Write  bool `json:"write,omitempty"`
	Close  bool `json:"close,omitempty"`
}

type InteractiveSnapshot struct {
	ArtifactID string         `json:"artifact_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type InteractiveObservation struct {
	NextOffset   int64               `json:"next_offset,omitempty"`
	Closed       bool                `json:"closed,omitempty"`
	ExitCode     *int                `json:"exit_code,omitempty"`
	Status       string              `json:"status,omitempty"`
	StatusReason string              `json:"status_reason,omitempty"`
	Snapshot     InteractiveSnapshot `json:"snapshot,omitempty"`
	UpdatedAt    int64               `json:"updated_at,omitempty"`
	Metadata     map[string]any      `json:"metadata,omitempty"`
}

type InteractiveOperation struct {
	Kind     InteractiveOperationKind `json:"kind,omitempty"`
	At       int64                    `json:"at,omitempty"`
	Offset   int64                    `json:"offset,omitempty"`
	Bytes    int64                    `json:"bytes,omitempty"`
	Metadata map[string]any           `json:"metadata,omitempty"`
}

type InteractiveRuntime struct {
	Handle        RuntimeHandle           `json:"handle"`
	Lineage       *RuntimeHandleLineage   `json:"lineage,omitempty"`
	Target        TargetRef               `json:"target,omitempty"`
	Capabilities  InteractiveCapabilities `json:"capabilities,omitempty"`
	Observation   InteractiveObservation  `json:"observation,omitempty"`
	LastOperation InteractiveOperation    `json:"last_operation,omitempty"`
	Metadata      map[string]any          `json:"metadata,omitempty"`
}

func InteractiveRuntimeFromHandle(handle RuntimeHandle) (InteractiveRuntime, bool) {
	if !isInteractiveHandle(handle) {
		return InteractiveRuntime{}, false
	}

	out := InteractiveRuntime{
		Handle:   handle,
		Metadata: cloneInteractiveMap(handle.Metadata),
		Observation: InteractiveObservation{
			UpdatedAt: maxRuntimeHandleTimestamp(handle),
		},
	}
	if ref, ok := TargetRefFromMetadata(handle.Metadata); ok {
		out.Target = ref
	}
	if lineage, ok := RuntimeHandleLineageFromHandle(handle); ok {
		lineageCopy := lineage
		out.Lineage = &lineageCopy
	}

	out.Capabilities = InteractiveCapabilities{
		Reopen: interactiveBool(handle.Metadata, InteractiveMetadataKeySupportsReopen),
		View:   interactiveBool(handle.Metadata, InteractiveMetadataKeySupportsView),
		Write:  interactiveBool(handle.Metadata, InteractiveMetadataKeySupportsWrite),
		Close:  interactiveBool(handle.Metadata, InteractiveMetadataKeySupportsClose),
	}
	if offset, ok := interactiveInt64(handle.Metadata, InteractiveMetadataKeyNextOffset); ok {
		out.Observation.NextOffset = offset
	}
	if closed := interactiveBool(handle.Metadata, InteractiveMetadataKeyClosed); closed {
		out.Observation.Closed = true
	}
	if exitCode, ok := interactiveInt(handle.Metadata, InteractiveMetadataKeyExitCode); ok {
		out.Observation.ExitCode = &exitCode
	}
	if status, _ := handle.Metadata[InteractiveMetadataKeyStatus].(string); status != "" {
		out.Observation.Status = status
	}
	if reason, _ := handle.Metadata[InteractiveMetadataKeyStatusReason].(string); reason != "" {
		out.Observation.StatusReason = reason
	}
	if artifactID, _ := handle.Metadata[InteractiveMetadataKeySnapshotArtifactID].(string); artifactID != "" {
		out.Observation.Snapshot = InteractiveSnapshot{ArtifactID: artifactID}
	}
	if out.Observation.Status == "" {
		out.Observation.Status = string(handle.Status)
	}
	if out.Observation.StatusReason == "" {
		out.Observation.StatusReason = handle.StatusReason
	}
	if handle.Status == RuntimeHandleClosed {
		out.Observation.Closed = true
	}

	if rawKind, _ := handle.Metadata[InteractiveMetadataKeyLastOperationKind].(string); rawKind != "" {
		out.LastOperation.Kind = InteractiveOperationKind(rawKind)
	}
	if at, ok := interactiveInt64(handle.Metadata, InteractiveMetadataKeyLastOperationAt); ok {
		out.LastOperation.At = at
	}
	if offset, ok := interactiveInt64(handle.Metadata, InteractiveMetadataKeyLastOperationOffset); ok {
		out.LastOperation.Offset = offset
	}
	if bytes, ok := interactiveInt64(handle.Metadata, InteractiveMetadataKeyLastOperationBytes); ok {
		out.LastOperation.Bytes = bytes
	}
	if out.LastOperation.Kind == "" && handle.Status == RuntimeHandleClosed && handle.ClosedAt > 0 {
		out.LastOperation.Kind = InteractiveOperationClose
		out.LastOperation.At = handle.ClosedAt
	}

	return out, true
}

func InteractiveRuntimesFromHandles(handles []RuntimeHandle) []InteractiveRuntime {
	out := make([]InteractiveRuntime, 0, len(handles))
	for _, handle := range handles {
		item, ok := InteractiveRuntimeFromHandle(handle)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func ApplyInteractiveRuntimeMetadata(metadata map[string]any, caps *InteractiveCapabilities, observation *InteractiveObservation, operation *InteractiveOperation) map[string]any {
	out := cloneInteractiveMap(metadata)
	if out == nil {
		out = map[string]any{}
	}
	out[InteractiveMetadataKeyEnabled] = true
	if caps != nil {
		out[InteractiveMetadataKeySupportsReopen] = caps.Reopen
		out[InteractiveMetadataKeySupportsView] = caps.View
		out[InteractiveMetadataKeySupportsWrite] = caps.Write
		out[InteractiveMetadataKeySupportsClose] = caps.Close
	}
	if observation != nil {
		out[InteractiveMetadataKeyNextOffset] = observation.NextOffset
		out[InteractiveMetadataKeyClosed] = observation.Closed
		if observation.ExitCode != nil {
			out[InteractiveMetadataKeyExitCode] = *observation.ExitCode
		}
		if observation.Status != "" {
			out[InteractiveMetadataKeyStatus] = observation.Status
		}
		if observation.StatusReason != "" {
			out[InteractiveMetadataKeyStatusReason] = observation.StatusReason
		}
		if observation.Snapshot.ArtifactID != "" {
			out[InteractiveMetadataKeySnapshotArtifactID] = observation.Snapshot.ArtifactID
		}
	}
	if operation != nil {
		if operation.Kind != "" {
			out[InteractiveMetadataKeyLastOperationKind] = string(operation.Kind)
		}
		if operation.At > 0 {
			out[InteractiveMetadataKeyLastOperationAt] = operation.At
		}
		if operation.Offset > 0 {
			out[InteractiveMetadataKeyLastOperationOffset] = operation.Offset
		}
		if operation.Bytes > 0 {
			out[InteractiveMetadataKeyLastOperationBytes] = operation.Bytes
		}
	}
	return out
}

func isInteractiveHandle(handle RuntimeHandle) bool {
	if len(handle.Metadata) == 0 {
		return handle.Kind == "pty"
	}
	if interactiveBool(handle.Metadata, InteractiveMetadataKeyEnabled) {
		return true
	}
	for _, key := range []string{
		InteractiveMetadataKeySupportsReopen,
		InteractiveMetadataKeySupportsView,
		InteractiveMetadataKeySupportsWrite,
		InteractiveMetadataKeySupportsClose,
		InteractiveMetadataKeyNextOffset,
		InteractiveMetadataKeyClosed,
		InteractiveMetadataKeyExitCode,
		InteractiveMetadataKeyStatus,
		InteractiveMetadataKeyStatusReason,
		InteractiveMetadataKeySnapshotArtifactID,
		InteractiveMetadataKeyLastOperationKind,
		InteractiveMetadataKeyLastOperationAt,
	} {
		if _, ok := handle.Metadata[key]; ok {
			return true
		}
	}
	if mode, _ := handle.Metadata["mode"].(string); mode == "pty" {
		return true
	}
	return handle.Kind == "pty"
}

func interactiveBool(metadata map[string]any, key string) bool {
	raw, ok := metadata[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	default:
		return false
	}
}

func interactiveInt64(metadata map[string]any, key string) (int64, bool) {
	raw, ok := metadata[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func interactiveInt(metadata map[string]any, key string) (int, bool) {
	value, ok := interactiveInt64(metadata, key)
	if !ok {
		return 0, false
	}
	return int(value), true
}

func cloneInteractiveMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func maxRuntimeHandleTimestamp(handle RuntimeHandle) int64 {
	maxTS := handle.UpdatedAt
	if handle.CreatedAt > maxTS {
		maxTS = handle.CreatedAt
	}
	if handle.ClosedAt > maxTS {
		maxTS = handle.ClosedAt
	}
	if handle.InvalidatedAt > maxTS {
		maxTS = handle.InvalidatedAt
	}
	return maxTS
}
