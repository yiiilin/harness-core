package session_test

import (
	"reflect"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestSessionStateDoesNotExposeParentSessionID(t *testing.T) {
	if _, ok := reflect.TypeOf(session.State{}).FieldByName("ParentSessionID"); ok {
		t.Fatalf("expected ParentSessionID to stay out of the stable kernel surface")
	}
}
