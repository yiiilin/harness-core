package session

// IsRecoverableState reports whether the session should be surfaced to
// recovery-oriented control paths such as ListRecoverableSessions and
// ClaimRecoverableSession.
func IsRecoverableState(st State) bool {
	switch st.ExecutionState {
	case ExecutionInFlight, ExecutionInterrupted:
		return true
	case ExecutionIdle:
		if st.Phase != PhaseRecover {
			return false
		}
		return st.InterruptedAt != 0 || st.InFlightStepID != ""
	default:
		return false
	}
}
