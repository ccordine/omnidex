package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestCausalAdmissionIgnoresEvidenceOutsideBoundedSurfaceResult(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceFilesystem, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	tampered := run.Episode.Manifest
	tampered.Trace, err = cloneTrace(run.Episode.Manifest.Trace)
	if err != nil {
		t.Fatal(err)
	}
	acquisitionID := fixture.generated.PrivateOracle().EvidenceUses[0].AcquisitionActionID
	changed := false
	for index := range tampered.Trace {
		entry := &tampered.Trace[index]
		if entry.Kind != TraceObservation {
			continue
		}
		observation := cognition.Observation{}
		if err := decodeTracePayload(entry.Payload, &observation, "test observation"); err != nil {
			t.Fatal(err)
		}
		if observation.ActionID != acquisitionID {
			continue
		}
		var envelope struct {
			Surface          string          `json:"surface"`
			Operation        string          `json:"operation"`
			SymbolicState    json.RawMessage `json:"symbolic_state"`
			SurfaceAuthority string          `json:"surface_authority"`
			Result           json.RawMessage `json:"result"`
		}
		if err := decodeStrictJSON([]byte(observation.Content), &envelope, "test surface"); err != nil {
			t.Fatal(err)
		}
		envelope.SymbolicState = envelope.Result
		envelope.Result = json.RawMessage(`{}`)
		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		observation, err = cognition.NewActionObservation(
			observation.ID, observation.ActionID, observation.Revision, observation.Kind, string(raw),
		)
		if err != nil {
			t.Fatal(err)
		}
		entry.Payload, err = traceJSONObject(observation)
		if err != nil {
			t.Fatal(err)
		}
		changed = true
		break
	}
	if !changed {
		t.Fatal("sealed episode lacked its declared acquisition observation")
	}
	tampered.TraceSHA256 = ""
	prepared, err := prepareEpisodeManifest(tampered)
	if err != nil {
		t.Fatal(err)
	}
	sealed := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	sealed.SealSHA256, err = digestJSON(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCausalAcquisitionTrace(
		fixture, sealed, run.Authority.SurfaceVersion,
	); err == nil {
		t.Fatal("causal admission counted evidence outside the bounded surface result")
	}
}

func TestCausalAdmissionMapsCodeAssignedActionIdentityByExactRequest(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest := run.Episode.Manifest
	manifest.Trace, err = cloneTrace(run.Episode.Manifest.Trace)
	if err != nil {
		t.Fatal(err)
	}
	witnessID := fixture.generated.PrivateOracle().EvidenceUses[0].AcquisitionActionID
	runtimeID := cognition.ActionID("runtime-action-acquisition")
	for index := range manifest.Trace {
		entry := &manifest.Trace[index]
		switch entry.Kind {
		case TraceAction:
			trace, err := decodeActionTrace(*entry, cognition.EpisodeRef{ID: manifest.EpisodeID})
			if err != nil {
				t.Fatal(err)
			}
			if trace.Action.ID != witnessID {
				continue
			}
			trace.Action.ID = runtimeID
			trace.Transition.ActionID = runtimeID
			for observationIndex := range trace.Transition.Observations {
				trace.Transition.Observations[observationIndex].ActionID = runtimeID
			}
			for effectIndex := range trace.Transition.Effects {
				trace.Transition.Effects[effectIndex].ActionID = runtimeID
			}
			entry.ID = string(runtimeID)
			entry.Payload = mustTraceJSONObject(t, trace)
		case TraceObservation:
			observation := cognition.Observation{}
			if err := decodeTracePayload(entry.Payload, &observation, "renamed observation"); err != nil {
				t.Fatal(err)
			}
			if observation.ActionID == witnessID {
				observation.ActionID = runtimeID
				entry.Payload = mustTraceJSONObject(t, observation)
			}
		}
	}
	manifest.TraceSHA256 = ""
	prepared, err := prepareEpisodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sealed := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	sealed.SealSHA256, err = digestJSON(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCausalAcquisitionTrace(
		fixture, sealed, run.Authority.SurfaceVersion,
	); err != nil {
		t.Fatalf("code-assigned action identity broke private causal mapping: %v", err)
	}
}
