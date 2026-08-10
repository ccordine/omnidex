package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRendererIsVersionedDeterministicAndUsesOnlyProjectedContext(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	first, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	second, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Version != RendererVersionV2 || first.SHA256 == "" || first.Bytes != len(first.JSON) {
		t.Fatalf("rendered envelope is not stable: %#v / %#v", first, second)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(first.JSON), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	projected, projectedOK := envelope["projected_context"].(map[string]any)
	if envelope["schema"] != EnvelopeSchemaV2 || envelope["renderer_version"] != RendererVersionV2 ||
		!projectedOK || projected["schema"] != "omnidex.context-material-json.v1" {
		t.Fatalf("envelope identity/context = %#v", envelope)
	}
	decisionSchema, ok := envelope["decision_schema"].(map[string]any)
	if !ok || decisionSchema["type"] != "object" || decisionSchema["additionalProperties"] != false {
		t.Fatalf("decision schema is not code-owned and strict: %#v", envelope["decision_schema"])
	}
	for _, forbidden := range []string{"transcript", "shell", "file_path", "workspace_path"} {
		if strings.Contains(strings.ToLower(first.JSON), forbidden) {
			t.Fatalf("envelope contains forbidden surface %q", forbidden)
		}
	}
}

func TestSerializedEnvelopeOmitsControlPlaneBindingsAndDuplicateState(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rendered.JSON), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"actor", "snapshot_sha256", "context_projection", "budget",
		"goal", "current_obligation", "current_revision",
	} {
		if _, exists := envelope[key]; exists {
			t.Fatalf("model envelope exposed code binding %q", key)
		}
	}
	for _, forbidden := range []string{
		`"job_id"`, `"step_id"`, `"attempt"`, `"worker_id"`,
		`"working_set_id"`, `"snapshot_sha256"`, `"context_projection"`,
	} {
		if strings.Contains(rendered.JSON, forbidden) {
			t.Fatalf("model envelope exposed orchestration field %s", forbidden)
		}
	}
	if strings.Contains(rendered.JSON, `"projected_context":"{`) {
		t.Fatal("projected context was embedded as an escaped JSON string")
	}
}

func TestRendererRejectsProjectionBeyondFixedInputLimit(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, strings.Repeat("x", MaxProjectedContextBytes+1))
	snapshot, _ := policyTestSnapshot(t, projection)
	if _, err := Render(snapshot, projection, policyTestBrain()); !errors.Is(err, ErrInputLimit) {
		t.Fatalf("error = %v, want ErrInputLimit", err)
	}
}

func TestBrainAndCallAttemptIdentitiesFailLoudly(t *testing.T) {
	t.Parallel()
	brain := policyTestBrain()
	if err := brain.Validate(); err != nil {
		t.Fatalf("validate brain: %v", err)
	}
	invalidBrain := brain
	invalidBrain.SamplingSHA256 = "bad"
	if err := invalidBrain.Validate(); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("brain error = %v, want ErrInvalidBrain", err)
	}

	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyAttestBrain(brain), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	attempt := journal.attempts[0]
	attempt.Brain.Hardware = "changed-hardware"
	if err := attempt.Validate(); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("attempt error = %v, want ErrInvalidEvidence", err)
	}
}

func TestCallIdentityBindsEveryBrainField(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	record := journal.attempts[0]
	mutations := map[string]func(*BrainRef){
		"model":        func(value *BrainRef) { value.Model = "model:changed" },
		"digest":       func(value *BrainRef) { value.Digest = strings.Repeat("d", 64) },
		"quantization": func(value *BrainRef) { value.Quantization = "q8_0" },
		"sampling": func(value *BrainRef) {
			value.Sampling.MaxOutputTokens++
			refreshPolicyTestSampling(value)
		},
		"native context": func(value *BrainRef) {
			value.NativeContextLimit++
			refreshPolicyTestSampling(value)
		},
		"byte ceiling": func(value *BrainRef) {
			value.ContextCeilingBytes++
			refreshPolicyTestSampling(value)
		},
		"backend": func(value *BrainRef) { value.Backend = "changed-backend" },
		"backend version": func(value *BrainRef) {
			value.BackendVersion = "2.0.0"
		},
		"hardware": func(value *BrainRef) { value.Hardware = "changed-hardware" },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := record
			mutate(&changed.Brain)
			if err := changed.Brain.Validate(); err != nil {
				t.Fatalf("mutation must remain a valid brain: %v", err)
			}
			if callAttemptID(changed) == record.ID {
				t.Fatal("call identity did not change")
			}
			if err := changed.Validate(); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("evidence error = %v, want ErrInvalidEvidence", err)
			}
		})
	}
}

func TestRendererEnforcesBoundBrainEnvelopeCeilings(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	brain := policyTestBrain()
	baseline, err := Render(snapshot, projection, brain)
	if err != nil {
		t.Fatal(err)
	}
	brain.ContextCeilingBytes = baseline.Bytes - 1
	refreshPolicyTestSampling(&brain)
	if _, err := Render(snapshot, projection, brain); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("byte ceiling error = %v, want ErrInvalidBrain", err)
	}
}

func TestMeasureEnvelopeIsExactAndBrainIndependent(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	measured, err := MeasureEnvelope(snapshot, projection)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	if measured != rendered || measured.Bytes != len(measured.JSON) ||
		measured.EstimatedTokens != estimatePolicyTokens(measured.Bytes) {
		t.Fatalf("measured=%+v rendered=%+v", measured, rendered)
	}
	invalidBrain := policyTestBrain()
	invalidBrain.Model = ""
	if _, err := Render(snapshot, projection, invalidBrain); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("Render invalid brain error=%v", err)
	}
	if _, err := MeasureEnvelope(snapshot, projection); err != nil {
		t.Fatalf("brain-independent measurement failed: %v", err)
	}
}
