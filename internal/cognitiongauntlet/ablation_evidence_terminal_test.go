package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

func TestAblationEvidenceRederivesAcceptedNoActionReason(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	request := ablationTestRequest(
		t, VariantRawShell, SurfaceFilesystem, 1,
		&witnessPolicyClient{model: mustRatGeneration(t).Fixed.Brain.Model},
	)
	client := &malformedRawShellPolicyClient{witnessPolicyClient: witnessPolicyClient{
		model: request.RatGeneration.Fixed.Brain.Model,
	}}
	request.Client = client
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := loadAblationEvidence(request.EvidenceSealPath, result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Root.NoActions) != 1 ||
		artifact.Root.NoActions[0].Reason != "raw_shell_parse_failure" {
		t.Fatalf("unexpected no-action authority: %+v", artifact.Root.NoActions)
	}
	artifact.Root.NoActions[0].Reason = "action_schema_absent"
	if err := verifyAblationEvidenceArtifact(artifact); err == nil {
		t.Fatal("accepted no-action reason was trusted instead of rederived")
	}
}

func TestAblationEvidenceRederivesNoCallTerminalCause(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	fixture.spec.Budget.ModelCalls = 1
	oracle := fixture.generated.PrivateOracle()
	request := ablationTestRequest(t, VariantRawObservation, SurfaceSymbolic, 1, &witnessPolicyClient{
		model: mustRatGeneration(t).Fixed.Brain.Model, witness: oracle.Witness,
		evidenceUses: oracle.EvidenceUses,
	})
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := loadAblationEvidence(request.EvidenceSealPath, result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Root.Terminal.FailureCode != "resource_budget" ||
		len(artifact.Root.Actions) != 1 || len(artifact.Root.NoActions) != 0 {
		t.Fatalf("unexpected budget terminal evidence: %+v", artifact.Root)
	}
	artifact.Root.Terminal.FailureCode = "invented_failure"
	artifact.Root.Terminal.PublicOutcome = "invented_failure"
	if err := verifyAblationEvidenceArtifact(artifact); err == nil {
		t.Fatal("no-call terminal cause was accepted without exact authority")
	}
}

func TestAblationEvidenceUsesRuntimeCyclePrecedenceAtEqualLimits(t *testing.T) {
	root := ablationEvidenceRoot{
		Calls: make([]ablationCallEvidence, 1),
		PublicRunAuthority: PublicRunAuthority{Budget: RunBudget{
			ModelCalls: 1, RuntimeCycles: 1,
		}},
		Terminal: ablationPendingTerminal{
			FailureCode: "resource_budget", PublicOutcome: "resource_budget",
		},
		TerminalCause: ablationTerminalCause{
			Kind: ablationTerminalCycleBudget, Reason: "runtime_cycles",
			CompletedCalls: 1, CompletedCycles: 1,
		},
	}
	if err := verifyAblationTerminalCause(root, false); err != nil {
		t.Fatalf("produced runtime-cycle cause rejected: %v", err)
	}
	root.TerminalCause.Kind = ablationTerminalPreCallBudget
	root.TerminalCause.Reason = "model_calls"
	if err := verifyAblationTerminalCause(root, false); err == nil {
		t.Fatal("equal model-call/runtime limits accepted the non-produced pre-call branch")
	}
	root.PublicRunAuthority.Budget.ModelCalls = 2
	root.TerminalCause.Kind = ablationTerminalContextBudget
	root.TerminalCause.Reason = "context_projection"
	root.ContextBudget = &ablationContextBudgetEvidence{}
	if err := verifyAblationTerminalCause(root, false); err == nil {
		t.Fatal("exhausted runtime-cycle limit accepted a never-executed context-budget branch")
	}
}

func TestAblationSemanticReplayUsesDurableBoundedPolicyFailure(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	base := &witnessPolicyClient{model: mustRatGeneration(t).Fixed.Brain.Model}
	request := ablationTestRequest(t, VariantRawObservation, SurfaceSymbolic, 1, base)
	request.Client = &terminalPolicyClient{
		witnessPolicyClient: base,
		failure: errors.New(strings.Repeat(
			"provider transport failure ", cognitionpolicy.MaxCallFailureBytes,
		)),
	}
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := loadAblationEvidence(request.EvidenceSealPath, result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Root.Calls) != 1 {
		t.Fatalf("policy failure calls=%d, want 1", len(artifact.Root.Calls))
	}
	want := artifact.Root.Calls[0].Result.FailureMessage
	if !strings.Contains(want, "exact byte SHA-256") || len(want) > cognitionpolicy.MaxCallFailureBytes {
		t.Fatalf("durable failure was not bounded: %q", want)
	}
	found := false
	for _, entry := range result.Episode.Manifest.Trace {
		if entry.Kind != TraceFailure {
			continue
		}
		var failure ablationFailureRecord
		if err := decodeTracePayload(entry.Payload, &failure, "bounded policy failure"); err != nil {
			t.Fatal(err)
		}
		found = true
		if failure.Message != want {
			t.Fatalf("trace failure differs from durable call result\ntrace=%q\nresult=%q", failure.Message, want)
		}
	}
	if !found {
		t.Fatal("policy failure did not emit its terminal trace")
	}
	bundle, err := NewVariantPublicInferenceBundle(
		fixture, result.Authority, VariantRawObservation,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExportAblationSemanticReplay(
		bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
	)
	if err != nil {
		t.Fatalf("bounded durable policy failure could not reopen: %v", err)
	}
}

func TestAblationSemanticReplayRecordsUnboundBudgetFailuresOnce(t *testing.T) {
	for _, kind := range []ablationTerminalCauseKind{
		ablationTerminalPreCallBudget, ablationTerminalCycleBudget,
	} {
		root := ablationEvidenceRoot{
			Obligation:    cognition.Obligation{ID: "root-obligation"},
			TerminalCause: ablationTerminalCause{Kind: kind},
			Terminal: ablationPendingTerminal{
				Revision: cognition.WorldRevision{Number: 1, SHA256: strings.Repeat("a", 64)},
			},
		}
		units, err := ablationSemanticUnits(root)
		if err != nil {
			t.Fatal(err)
		}
		failures := 0
		for _, unit := range units {
			for _, event := range unit.events {
				if event == cognitionreplay.EventFailureRecorded {
					failures++
				}
			}
		}
		if failures != 1 {
			t.Fatalf("terminal cause %q emitted %d failure events, want 1", kind, failures)
		}
	}
}

type malformedRawShellPolicyClient struct {
	witnessPolicyClient
}

func (client *malformedRawShellPolicyClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if err := client.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedGeneration{}, err
	}
	var envelope witnessPolicyEnvelope
	if err := json.Unmarshal([]byte(prepared.Prompt), &envelope); err != nil {
		return llm.PreparedGeneration{}, err
	}
	obligation, err := witnessCurrentObligation(envelope)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	action, err := cognition.NewActionRequest("shell", []cognition.ActionArgument{{
		Name: rawShellArgument, Value: "not-a-registered-command",
	}})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	raw, err := json.Marshal(cognition.CognitionDecision{
		ObligationID: obligation.ID, Action: action,
		EvidenceRefs:   []cognition.EvidenceRef{},
		ExpectedEffect: "Apply the registered transition described by the bounded action.",
		Proposals:      []cognition.LedgerProposal{}, Attention: []cognition.AttentionRequest{},
	})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return client.exactPreparedGeneration(prepared, string(raw))
}
