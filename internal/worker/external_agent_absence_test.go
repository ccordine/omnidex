package worker

import (
	"os"
	"strings"
	"testing"
)

func TestExternalAgentMutationRuntimeIsAbsent(t *testing.T) {
	if _, err := os.Stat("external_agent.go"); err == nil {
		t.Fatal("external agent whole-workspace mutation runtime remains")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	source, err := os.ReadFile("step_runner.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"external_agent_execute", "runExternalAgentStep", "selectExternalAgent"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("worker dispatcher retains external agent authority %q", forbidden)
		}
	}
}
