package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTerminalPlanningRequiresOpeningAfterPreviousCallCompletion(t *testing.T) {
	call := terminalSemanticTranscriptFixture(t, assemblyline.WorkApplicationRequirementCandidateAuthorization)
	opening, completed, _, err := terminalSemanticFacts(call)
	if err != nil {
		t.Fatal(err)
	}
	var activity terminalWorkflowActivity
	activity.observe(t, terminalFactBlock(opening, 0)+terminalFactBlock(opening, 0)+terminalFactBlock(completed, call.id)+terminalFactBlock(completed, call.id+1))
	used := make(map[int]bool)
	opened, visible, err := activity.planningCallVisible(call, used)
	if err != nil || !visible {
		t.Fatalf("first actual call was not visible: %v", err)
	}
	used[opened] = true
	call.id++
	if _, visible, err := activity.planningCallVisible(call, used); err != nil || visible {
		t.Fatalf("later call reused an earlier duplicate opening: visible=%t err=%v", visible, err)
	}
}

func TestTerminalPlanningIgnoresCallLabelsInsideReturnedContent(t *testing.T) {
	call := terminalSemanticTranscriptFixture(t, assemblyline.WorkApplicationRequirementInventory)
	call.result = "The user can cancel a pending request.\nomni> remains returned content.\nModel: returned-content\nCall: 999\nThe existing state is preserved."
	opening, completed, _, err := terminalSemanticFacts(call)
	if err != nil {
		t.Fatal(err)
	}
	var activity terminalWorkflowActivity
	activity.observe(t, terminalFactBlock(opening, 0)+terminalFactBlock(completed, call.id))
	activity.assertSemanticCalls(t, []terminalSemanticCall{call})
}

func TestTerminalPlanningReconstructsAllResultPagesWithRepeatedCallID(t *testing.T) {
	call := terminalSemanticTranscriptFixture(t, assemblyline.WorkApplicationRequirementInventory)
	call.result = strings.TrimSpace(strings.Repeat("The user can cancel an existing pending request without changing accepted state.\n", 70))
	opening, completed, _, err := terminalSemanticFacts(call)
	if err != nil {
		t.Fatal(err)
	}
	first := append([]string(nil), completed...)
	first[1] = "Result: " + call.result[:4096]
	continued := fmt.Sprintf("Result continued (bytes %d–%d of %d):\n%s", 4097, len(call.result), len(call.result), call.result[4096:])
	var activity terminalWorkflowActivity
	activity.observe(t, terminalFactBlock(opening, 0)+terminalFactBlock(first, call.id)+terminalFactBlock([]string{continued, "Model: " + call.model}, call.id))
	activity.assertSemanticCalls(t, []terminalSemanticCall{call})
}

func terminalSemanticTranscriptFixture(t *testing.T, kind assemblyline.WorkKind) terminalSemanticCall {
	t.Helper()
	const request = "The user can cancel a pending request."
	context, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	var job assemblyline.PortableJob
	if kind == assemblyline.WorkApplicationRequirementInventory {
		job, err = assemblyline.NewApplicationRequirementInventoryJob(assemblyline.ApplicationRequirementInventoryInput{UserRequest: request, Context: context})
	} else {
		job, err = assemblyline.NewApplicationRequirementCandidateAuthorizationJob(assemblyline.ApplicationRequirementCandidateAuthorizationInput{UserRequest: request, Context: context, Candidate: "A pending request can be canceled."})
	}
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return terminalSemanticCall{id: 17, step: 1, attempt: 1, kind: string(job.Kind), model: "fixture-model", input: prompt, payload: job.Payload, result: "A"}
}

func terminalFactBlock(facts []string, callID int64) string {
	var block strings.Builder
	fmt.Fprintln(&block, "omni> "+strings.ReplaceAll(facts[0], "\n", "\n      "))
	model := ""
	for _, fact := range facts[1:] {
		if strings.HasPrefix(fact, "Model: ") {
			model = fact
			continue
		}
		fmt.Fprintln(&block, "      "+strings.ReplaceAll(fact, "\n", "\n      "))
	}
	fmt.Fprintln(&block, "      "+model)
	if callID > 0 {
		fmt.Fprintf(&block, "      Call: %d\n", callID)
	}
	return block.String()
}
