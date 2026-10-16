package codex

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

type capturedPlanningCase struct {
	SharedContext string `json:"shared_context"`
	Instruction   string `json:"instruction"`
	Responses     []struct {
		OwnerTurn string `json:"owner_turn"`
		Response  string `json:"response"`
	} `json:"responses"`
	Ledger   codingagent.EngineeringLedger `json:"base_ledger"`
	Rejected struct {
		Operations []map[string]json.RawMessage `json:"operations"`
	} `json:"mutation_parser_response"`
}

func capturedPlanningFixture(t *testing.T, name string) capturedPlanningCase {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture capturedPlanningCase
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestPlanningMutationSchemaCannotRequireMixedOperationFields(t *testing.T) {
	schema := agentOutputSchema(codingagent.Assignment{Role: codingagent.PlanningMutation})
	variants := schema["properties"].(map[string]any)["operations"].(map[string]any)["items"].(map[string]any)["anyOf"].([]map[string]any)
	for _, name := range []string{"expense", "wordcounter", "preservation"} {
		fixture := capturedPlanningFixture(t, name)
		for _, operation := range fixture.Rejected.Operations {
			var kind string
			_ = json.Unmarshal(operation["kind"], &kind)
			for _, variant := range variants {
				properties := variant["properties"].(map[string]any)
				if properties["kind"].(map[string]any)["enum"].([]string)[0] != kind {
					continue
				}
				if kind == "add" || kind == "amend" {
					if properties["review_state"] != nil || properties["rank"] != nil {
						t.Fatal("parser still forced to populate forbidden mutation fields")
					}
					if kind == "add" && properties["target_id"] != nil {
						t.Fatal("add still forced to reconstruct target_id")
					}
				}
			}
		}
	}
}

func TestPlanningMutationSchemaRejectsBlankTargets(t *testing.T) {
	schema := agentOutputSchema(codingagent.Assignment{Role: codingagent.PlanningMutation})
	variants := schema["properties"].(map[string]any)["operations"].(map[string]any)["items"].(map[string]any)["anyOf"].([]map[string]any)
	for _, variant := range variants {
		properties := variant["properties"].(map[string]any)
		if target, exists := properties["target_id"]; exists && target.(map[string]any)["pattern"] != `\S` {
			t.Fatal("parser can still emit blank operation targets")
		}
	}
}

type replayPlanningJournal struct{ entries []codingagent.Entry }

func (j *replayPlanningJournal) Append(_ context.Context, e codingagent.Entry) (int64, error) {
	e.ID = int64(len(j.entries) + 1)
	j.entries = append(j.entries, e)
	return e.ID, nil
}
func (j *replayPlanningJournal) Read(context.Context) ([]codingagent.Entry, error) {
	return j.entries, nil
}
func (j *replayPlanningJournal) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}

type replayPlanningEnvironment struct{}

func (replayPlanningEnvironment) Attest(context.Context) error { return nil }

type replayPlanningProvider struct {
	client *Client
	t      *testing.T
}

func (provider replayPlanningProvider) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	started := time.Now()
	result, err := provider.client.Run(ctx, call, func(value codingagent.NativeEvent) error {
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"params"`
		}
		if json.Unmarshal(value.Frame, &frame) == nil && frame.Method == "error" {
			message := frame.Params.Error.Message
			if len(message) > 1000 {
				message = message[:1000]
			}
			provider.t.Logf("%s provider error: %s", call.Role, message)
		}
		return event(value)
	})
	provider.t.Logf("%s elapsed=%s completed=%v error=%v", call.Role, time.Since(started).Round(time.Millisecond), result.Completed, err)
	return result, err
}

// These are explicit installed-model regression replays, not a claim that an
// application was built. Production renderers parse the exact captured prose;
// expected ledger objects never enter provider context.
func TestInstalledCodexCapturedPlanningFailures(t *testing.T) {
	client, model := installedAgentClient(t)
	for _, name := range []string{"expense", "wordcounter", "preservation"} {
		t.Run(name, func(t *testing.T) {
			fixture := capturedPlanningFixture(t, name)
			route := codingagent.Route{Provider: "codex", Model: model, Effort: "low", ServiceTier: "default"}
			journal := &replayPlanningJournal{}
			t.Cleanup(func() {
				if !t.Failed() {
					return
				}
				for _, entry := range journal.entries {
					if entry.Phase == "call_completed" && entry.Result != nil {
						t.Logf("captured_failed_replay role=%s content=%s", entry.Role, entry.Result.Content)
					}
				}
			})
			controller := &codingagent.Controller{JobID: 17, Generation: 1, Root: t.TempDir(),
				Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
				Providers:     map[string]codingagent.Provider{"codex": replayPlanningProvider{client: client, t: t}}, Evidence: journal, Environment: replayPlanningEnvironment{},
				Requirements: []codingagent.Requirement{{ID: "objective", Outcome: fixture.Instruction}},
			}
			if fixture.SharedContext != "" {
				controller.MinifyContext = func(_ context.Context, call codingagent.Assignment) (string, error) {
					if call.Role == codingagent.PlanningMutation {
						return fixture.SharedContext, nil
					}
					return "", nil
				}
			}
			turn := codingagent.PlanningTurn{OperationID: "captured-initial", OwnerTurnID: "owner-initial", Text: fixture.Responses[0].OwnerTurn}
			journal.entries = []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, fixture.Responses[0].Response}}}
			proposal, err := controller.ProposePlanningLedger(context.Background(), turn)
			if err != nil {
				t.Fatal(err)
			}
			if err := proposal.Ledger.ValidateDraft(); err != nil {
				t.Fatal(err)
			}
			if name == "wordcounter" {
				if len(proposal.Ledger.Tasks) == 0 {
					t.Fatal("implementation tasks were lost")
				}
				if err := proposal.Ledger.Validate(); err != nil {
					t.Fatalf("captured implementation plan still cannot harden: %v", err)
				}
			}
			turn = codingagent.PlanningTurn{OperationID: "captured-mutation", OwnerTurnID: "owner-mutation", Text: fixture.Responses[1].OwnerTurn, ObservedRevision: fixture.Ledger.Revision}
			journal.entries = []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, fixture.Responses[1].Response}}}
			mutation, err := controller.PlanPlanningMutation(context.Background(), fixture.Ledger, turn)
			if err != nil {
				t.Fatal(err)
			}
			next, err := codingagent.ApplyPlanningMutation(fixture.Ledger, mutation)
			if err != nil {
				t.Fatalf("captured conversational update still fails: %v", err)
			}
			for _, before := range fixture.Ledger.Tasks {
				for _, after := range next.Tasks {
					if after.ID == before.ID && (after.Goal != before.Goal || !reflect.DeepEqual(after.RequirementIDs, before.RequirementIDs) || !reflect.DeepEqual(after.ConstraintIDs, before.ConstraintIDs) || !reflect.DeepEqual(after.WriteFiles, before.WriteFiles) || after.Provenance != before.Provenance || after.Rank != before.Rank) {
						t.Fatalf("parser stomped preserved task %s: before=%+v after=%+v", before.ID, before, after)
					}
				}
			}
			if name == "preservation" {
				obligation := next.Tasks[1].DeliveryObligation
				for _, expected := range []string{fixture.Ledger.Tasks[1].DeliveryObligation, "d720deeea8d9732c3267b261ba30e5dea716e5b0939b9eb1159e4623727666ef", "f2caaf7c165dbc7de127703d47963bbd3adeb3d05e23462fa16881e389be49de"} {
					if !strings.Contains(obligation, expected) {
						t.Fatal("parsed task context lost a baseline or existing obligation")
					}
				}
			}
			t.Logf("captured initial and mutation replies parsed: tasks=%d criteria=%d checks=%d operations=%d", len(proposal.Ledger.Tasks), len(proposal.Ledger.AcceptanceCriteria), len(proposal.Ledger.Checks), len(mutation.Operations))
		})
	}
}
