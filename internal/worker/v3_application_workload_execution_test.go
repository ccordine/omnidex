package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestCapabilityPairsStaySeparateFromWorkloadSchedulingAndLeafAuthority(t *testing.T) {
	t.Parallel()

	input := workerApplicationWorkloadInput()
	input.ProductQuote = "records console PRODUCT_ONLY_MARKER"
	frozen, err := assemblyline.FreezeApplicationWorkload(input, workerApplicationWorkloadDraft())
	if err != nil {
		t.Fatal(err)
	}
	wavesBefore, err := assemblyline.BuildApplicationWorkloadWaves(input, frozen)
	if err != nil {
		t.Fatal(err)
	}
	wantWaves := [][]string{{"task_001", "task_002", "task_003"}}
	if !reflect.DeepEqual(wavesBefore, wantWaves) {
		t.Fatalf("materialized workload waves=%v want %v", wavesBefore, wantWaves)
	}

	pairs := directCodingCapabilityPairs(input.ProductQuote, input.Requirements)
	if len(pairs) != 3 {
		t.Fatalf("capability pairs=%d want 3", len(pairs))
	}
	var capabilityPrompts []string
	capabilityRuntime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, MaxConcurrency: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, schema map[string]any) (string, error) {
			if schema == nil {
				return "", fmt.Errorf("capability relation requires a closed semantic schema")
			}
			capabilityPrompts = append(capabilityPrompts, prompt)
			relation := assemblyline.CapabilityIndependent
			if len(capabilityPrompts) == 1 {
				relation = assemblyline.CapabilityRightReadsLeft
			}
			return fmt.Sprintf(`{"schema":%q,"relation":%q}`, assemblyline.CapabilityRelationSchemaV1, relation), nil
		}),
	}
	results := runDirectCodingCapabilityPairs(capabilityRuntime, "semantic", pairs)
	capabilities, err := assembleDirectCodingCapabilityGraph(input.Requirements, results)
	if err != nil {
		t.Fatal(err)
	}
	for index, prompt := range capabilityPrompts {
		pair := pairs[index]
		if strings.Count(prompt, pair.Input.LeftNeed) != 1 || strings.Count(prompt, pair.Input.RightNeed) != 1 {
			t.Fatalf("capability pair %d did not see exactly two local needs:\n%s", index, prompt)
		}
		for requirementIndex, requirement := range input.Requirements {
			if requirementIndex != pair.LeftIndex && requirementIndex != pair.RightIndex &&
				strings.Contains(prompt, requirement.SourceQuote) {
				t.Fatalf("capability pair %d saw third need %q:\n%s", index, requirement.SourceQuote, prompt)
			}
		}
		for _, forbidden := range []string{"task_001", "requirement_001", "workload", "execution order", "next task"} {
			if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
				t.Fatalf("capability pair gained scheduler authority through %q:\n%s", forbidden, prompt)
			}
		}
	}
	if got := capabilities["requirement_002"]; len(got) != 1 || got[0].Purpose != "groups records" {
		t.Fatalf("capability graph did not retain direct live-state relation: %+v", capabilities)
	}
	wavesAfter, err := assemblyline.BuildApplicationWorkloadWaves(input, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wavesAfter, wantWaves) {
		t.Fatalf("capability relation mutated workload scheduling: before=%v after=%v", wavesBefore, wavesAfter)
	}
	for _, task := range frozen.Tasks {
		if len(task.DependsOn) != 0 {
			t.Fatalf("capability relation became scheduler dependency: %+v", task)
		}
	}

	var order []string
	prompts := make(map[string]string)
	fragmentRuntime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, MaxConcurrency: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, schema map[string]any) (string, error) {
			if schema != nil {
				t.Fatalf("source leaf unexpectedly used a semantic response schema: %#v", schema)
			}
			const marker = "The declaration must match this signature exactly:\n"
			_, remainder, found := strings.Cut(prompt, marker)
			if !found {
				return "", fmt.Errorf("source prompt omitted its code-owned signature")
			}
			signature, _, _ := strings.Cut(remainder, "\n")
			prompts[taskIDFromLeafSignature(signature)] = prompt
			return signature + " { return 1; }", nil
		}),
	}
	err = executeDirectCodingApplicationWorkload(input, frozen, func(taskContext assemblyline.ApplicationTaskContext) error {
		raw, marshalErr := json.Marshal(taskContext)
		if marshalErr != nil {
			return marshalErr
		}
		taskID, decodeErr := applicationTaskID(raw)
		if decodeErr != nil {
			return decodeErr
		}
		projected, projectErr := assemblyline.ProjectApplicationTaskContext(input, frozen, taskID)
		if projectErr != nil || !reflect.DeepEqual(taskContext, projected) {
			return fmt.Errorf("executor changed projected task %s: %v", taskID, projectErr)
		}
		behavior, behaviorErr := compileDirectCodingApplicationTaskBehavior(
			taskContext, capabilities[taskContext.Task.RequirementID],
		)
		if behaviorErr != nil {
			return behaviorErr
		}
		order = append(order, taskID)
		sequence := strings.TrimPrefix(taskID, "task_")
		_, workerErr := runDirectCodingTypeScriptFragmentWorker(
			fragmentRuntime, "coder", directCodingTypeScriptFragmentJob{block: assemblyline.TypeScriptBlock{
				ID: taskID, Signature: fmt.Sprintf("function leaf%s(): number", sequence),
				Contract: behavior, API: fmt.Sprintf("function leaf%s(): number", sequence),
			}},
		)
		return workerErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"task_001", "task_002", "task_003"}) {
		t.Fatalf("code-owned execution order=%v", order)
	}

	assertMinimalLeafPrompt(t, prompts["task_001"], []string{
		string(input.Surface), input.ProductQuote, "groups records",
		"Implement interactive record grouping in the records console.",
		"Users can create a named group.",
		"Users can assign and remove visible records from a group.",
		"A newly created named group is visible.",
		"Assigning a record visibly lists it in the selected group.",
	}, []string{frozen.SHA256, "task_", "requirement_", "filters records", "exports summaries"})
	assertMinimalLeafPrompt(t, prompts["task_003"], []string{
		string(input.Surface), input.ProductQuote, "exports summaries",
		"Implement summary export from the records console.",
		"Users can request an export of the visible record summary.",
		"The export represents the current visible record collection.",
		"The export action produces a summary artifact.",
		"The produced summary reflects the records visible when export was requested.",
	}, []string{frozen.SHA256, "task_", "requirement_", "groups records", "filters records"})
	assertMinimalLeafPrompt(t, prompts["task_002"], []string{
		string(input.Surface), input.ProductQuote, "filters records",
		"Implement interactive record filtering in the records console.",
		"Users can enter and clear a record filter.",
		"The visible record collection responds to the active filter.",
		"Entering a filter hides records that do not match it.",
		"Clearing the filter restores the full visible record collection.",
		"groups records",
	}, []string{
		frozen.SHA256, "task_", "requirement_", "exports summaries",
		"Implement interactive record grouping in the records console.",
		"Users can create a named group.", "A newly created named group is visible.",
	})
}

func TestApplicationWorkloadExecutionRejectsMutationBeforeAnyLeafStarts(t *testing.T) {
	t.Parallel()
	input := workerApplicationWorkloadInput()
	frozen, err := assemblyline.FreezeApplicationWorkload(input, workerApplicationWorkloadDraft())
	if err != nil {
		t.Fatal(err)
	}
	mutated := frozen
	mutated.Tasks = append([]assemblyline.FrozenApplicationTask(nil), frozen.Tasks...)
	mutated.Tasks[0].Objective = "Changed after freeze."
	calls := 0
	err = executeDirectCodingApplicationWorkload(input, mutated, func(assemblyline.ApplicationTaskContext) error {
		calls++
		return nil
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hash") || calls != 0 {
		t.Fatalf("mutated workload error=%v leaf_calls=%d", err, calls)
	}
}

func workerApplicationWorkloadDraft() assemblyline.ApplicationWorkloadDraft {
	return assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{
			{
				RequirementID: "requirement_001",
				Objective:     "Implement interactive record grouping in the records console.",
				RequiredBehaviors: []string{
					"Users can create a named group.",
					"Users can assign and remove visible records from a group.",
				},
				AcceptanceCriteria: []string{
					"A newly created named group is visible.",
					"Assigning a record visibly lists it in the selected group.",
				},
			},
			{
				RequirementID: "requirement_002",
				Objective:     "Implement interactive record filtering in the records console.",
				RequiredBehaviors: []string{
					"Users can enter and clear a record filter.",
					"The visible record collection responds to the active filter.",
				},
				AcceptanceCriteria: []string{
					"Entering a filter hides records that do not match it.",
					"Clearing the filter restores the full visible record collection.",
				},
			},
			{
				RequirementID: "requirement_003",
				Objective:     "Implement summary export from the records console.",
				RequiredBehaviors: []string{
					"Users can request an export of the visible record summary.",
					"The export represents the current visible record collection.",
				},
				AcceptanceCriteria: []string{
					"The export action produces a summary artifact.",
					"The produced summary reflects the records visible when export was requested.",
				},
			},
		},
	}
}

func applicationTaskID(raw []byte) (string, error) {
	var projection struct {
		Task struct {
			TaskID string `json:"task_id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(raw, &projection); err != nil {
		return "", err
	}
	if projection.Task.TaskID == "" {
		return "", fmt.Errorf("projected application task has no task_id: %s", raw)
	}
	return projection.Task.TaskID, nil
}

func taskIDFromLeafSignature(signature string) string {
	name := strings.TrimPrefix(strings.TrimSuffix(strings.Fields(signature)[1], "():"), "leaf")
	return "task_" + name
}

func assertMinimalLeafPrompt(t *testing.T, prompt string, required, forbidden []string) {
	t.Helper()
	if prompt == "" {
		t.Fatal("missing leaf prompt")
	}
	for _, value := range required {
		if !strings.Contains(prompt, value) {
			t.Fatalf("leaf prompt omitted %q:\n%s", value, prompt)
		}
	}
	for _, value := range forbidden {
		if strings.Contains(prompt, value) {
			t.Fatalf("leaf prompt exposed %q:\n%s", value, prompt)
		}
	}
}
