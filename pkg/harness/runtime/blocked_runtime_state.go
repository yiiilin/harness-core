package runtime

import "github.com/yiiilin/harness-core/pkg/harness/session"

const (
	sessionBlockedRuntimeIDKey = "_kernel_blocked_runtime_id"
)

func currentBlockedRuntimeID(st session.State) string {
	if len(st.Metadata) == 0 {
		return ""
	}
	blockedRuntimeID, _ := st.Metadata[sessionBlockedRuntimeIDKey].(string)
	return blockedRuntimeID
}

func hasCurrentBlockedRuntime(st session.State) bool {
	return currentBlockedRuntimeID(st) != ""
}

func setCurrentBlockedRuntime(st session.State, blockedRuntimeID string) session.State {
	next := st
	next.Metadata = cloneAnyMap(st.Metadata)
	if blockedRuntimeID == "" {
		delete(next.Metadata, sessionBlockedRuntimeIDKey)
		return next
	}
	next.Metadata[sessionBlockedRuntimeIDKey] = blockedRuntimeID
	return next
}
