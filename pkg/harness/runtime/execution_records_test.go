package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

func TestRunStepPersistsExecutionFactsAndRichEventEnvelope(t *testing.T) {
	rt, sess, step := newHappyRuntime()

	out, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	attempts := rt.ListAttempts(sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one attempt record, got %#v", attempts)
	}
	attempt := attempts[0]
	if attempt.AttemptID == "" || attempt.TaskID == "" || attempt.TraceID == "" {
		t.Fatalf("expected attempt identifiers to be populated, got %#v", attempt)
	}

	actions := rt.ListActions(sess.SessionID)
	if len(actions) != 1 {
		t.Fatalf("expected one action record, got %#v", actions)
	}
	actionRec := actions[0]
	if actionRec.ActionID == "" || actionRec.AttemptID != attempt.AttemptID || actionRec.TraceID != attempt.TraceID {
		t.Fatalf("expected action record to link to attempt, got %#v", actionRec)
	}

	verifications := rt.ListVerifications(sess.SessionID)
	if len(verifications) != 1 {
		t.Fatalf("expected one verification record, got %#v", verifications)
	}
	verifyRec := verifications[0]
	if verifyRec.VerificationID == "" || verifyRec.AttemptID != attempt.AttemptID || verifyRec.TraceID != attempt.TraceID {
		t.Fatalf("expected verification record to link to attempt, got %#v", verifyRec)
	}

	artifacts := rt.ListArtifacts(sess.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected at least one artifact record for execution output")
	}

	for _, event := range out.Events {
		if event.TaskID == "" || event.AttemptID == "" || event.TraceID == "" {
			t.Fatalf("expected event envelope ids on every event, got %#v", event)
		}
		switch event.Type {
		case audit.EventToolCalled, audit.EventToolCompleted, audit.EventToolFailed:
			if event.ActionID == "" {
				t.Fatalf("expected action_id on tool event, got %#v", event)
			}
			if event.CausationID == "" {
				t.Fatalf("expected causation_id on tool event, got %#v", event)
			}
		case audit.EventVerifyCompleted:
			if event.CausationID == "" {
				t.Fatalf("expected causation_id on verify event, got %#v", event)
			}
		}
	}
}
