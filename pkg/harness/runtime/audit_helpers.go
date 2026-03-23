package runtime

import (
	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

func newAuditEventAt(now int64, eventType, sessionID, taskID, stepID string, payload map[string]any) audit.Event {
	return audit.Event{
		EventID:   "evt_" + uuid.NewString(),
		Type:      eventType,
		SessionID: sessionID,
		TaskID:    taskID,
		StepID:    stepID,
		Payload:   payload,
		CreatedAt: now,
	}
}
