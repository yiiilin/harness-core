package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWorkflowScenariosExampleOutput(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run workflow scenarios example: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, needle := range []string{
		"scenario: planner-pipe",
		"scenario: approval-resume",
		"scenario: recover-interrupted",
		"stdout: planner walkthrough",
		"stdout: approval walkthrough",
		"stdout: recovery walkthrough",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, text)
		}
	}
}
