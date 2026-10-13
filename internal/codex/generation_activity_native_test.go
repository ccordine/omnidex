package codex

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

// Opt-in qualification observes a real, tool-free semantic turn through the
// installed sign-in. It adds no model-facing progress/summary instructions.
func TestInstalledCodexSemanticAvailablePublicActivityBeforeFinalResult(t *testing.T) {
	command, modelName := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL")
	if command == "" || modelName == "" {
		t.Skip("installed semantic activity qualification not selected")
	}
	client, err := New(command, ".", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	input, prompt := semanticRequirementInventoryRequest(t)
	prepared := nativeTestRequest(modelName, prompt)
	prepared.RetainProviderContext = false
	returned := false
	publicEvents, publicBytes := 0, 0
	threadID, turnID := "", ""
	result, err := client.GeneratePreparedExactWithActivity(context.Background(), prepared, func(activity model.AgentActivity) error {
		if returned {
			return fmt.Errorf("actual semantic public activity arrived after the result")
		}
		if err := activity.Validate(); err != nil {
			return err
		}
		if activity.Source != model.AgentActivityNative || activity.Role != "plan" || activity.TaskID != "" || (activity.Kind != model.AgentActivityMessage && activity.Kind != model.AgentActivityReasoningSummary) {
			return fmt.Errorf("actual semantic activity changed its public observation scope")
		}
		if threadID == "" {
			threadID, turnID = activity.ThreadID, activity.TurnID
		}
		if activity.ThreadID != threadID || activity.TurnID != turnID {
			return fmt.Errorf("actual semantic activity changed its original native identity")
		}
		if activity.Text != "" {
			publicEvents++
			publicBytes += len(activity.Text)
		}
		return nil
	})
	returned = true
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, result); err != nil {
		t.Fatal(err)
	}
	inventory, err := assemblyline.DecodeApplicationRequirementInventory(input, result.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) < 2 {
		t.Fatalf("actual production requirement inventory omitted independent outcomes: candidates=%d", len(inventory.Candidates))
	}
	retained, err := llm.DecodeCodexRetainedContext(result.ProviderResponseCapture)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := llm.ParseCodexContinuation(retained)
	if err != nil || identity.Model != modelName || (publicEvents > 0 && (identity.ThreadID != threadID || identity.TurnID != turnID)) {
		t.Fatalf("actual semantic public identity differs from its original native capture: %v", err)
	}
	if publicEvents == 0 {
		t.Logf("native inventory emitted no public text; this sample does not qualify live summary availability; nativeusage=%+v", result.Usage)
	} else {
		t.Logf("actual production inventory/public activity before final result: candidates=%d events=%d publicbytes=%d nativeusage=%+v", len(inventory.Candidates), publicEvents, publicBytes, result.Usage)
	}
}

func semanticRequirementInventoryRequest(t testing.TB) (assemblyline.ApplicationRequirementInventoryInput, string) {
	t.Helper()
	const request = "Build software that lets a user add named items, rename a selected item, remove a selected item, and undo the most recent change. Names and their order must survive closing and reopening the software. The user can filter items by a supplied part of their name, and clearing the filter restores the complete order."
	established, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ApplicationRequirementInventoryInput{UserRequest: request, Context: established}
	job, err := assemblyline.NewApplicationRequirementInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return input, prompt
}
