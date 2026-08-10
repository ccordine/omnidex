package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

const CausalAcquisitionReportSchemaV1 = "omnidex.causal-acquisition-report.v1"

type CausalAcquisitionReport struct {
	Schema               string   `json:"schema"`
	EpisodeSealSHA256    string   `json:"episode_seal_sha256"`
	OracleSHA256         string   `json:"oracle_sha256"`
	SurfaceVersion       string   `json:"surface_version"`
	EvidenceUseSHA256    string   `json:"evidence_use_sha256"`
	RequiredEvidence     int      `json:"required_evidence"`
	AcquiredEvidence     int      `json:"acquired_evidence"`
	AcquisitionTraceRefs []string `json:"acquisition_trace_refs"`
}

func ValidateCausalAcquisitionTrace(
	fixture MicrogauntletCase,
	episode SealedEpisode,
	surfaceVersion string,
) (CausalAcquisitionReport, error) {
	report, err := MeasureCausalAcquisitionTrace(fixture, episode, surfaceVersion)
	if err != nil {
		return CausalAcquisitionReport{}, err
	}
	if report.AcquiredEvidence != report.RequiredEvidence {
		return CausalAcquisitionReport{}, fmt.Errorf(
			"sealed episode causally acquired %d of %d required evidence records",
			report.AcquiredEvidence, report.RequiredEvidence,
		)
	}
	return report, nil
}

// MeasureCausalAcquisitionTrace preserves failed and partial episodes without
// granting them competence. Only an exact acquisition observation followed by
// its registered later consumer counts as acquired evidence.
func MeasureCausalAcquisitionTrace(
	fixture MicrogauntletCase,
	episode SealedEpisode,
	surfaceVersion string,
) (CausalAcquisitionReport, error) {
	if err := episode.Validate(); err != nil {
		return CausalAcquisitionReport{}, err
	}
	if err := requireExact(surfaceVersion, "causal acquisition surface version", 256); err != nil {
		return CausalAcquisitionReport{}, err
	}
	oracle := fixture.generated.PrivateOracle()
	if episode.Manifest.Scenario != fixture.generated.ExecutionScenario().Ref() {
		return CausalAcquisitionReport{}, fmt.Errorf("causal acquisition episode changed its scenario")
	}
	if err := validateEvidenceUseContract(
		fixture.spec.Generator.Suite, oracle, fixture.generated.PublicArtifact(),
	); err != nil {
		return CausalAcquisitionReport{}, err
	}
	observations, err := acquisitionObservations(episode)
	if err != nil {
		return CausalAcquisitionReport{}, err
	}
	bindings, err := tracedActionBindings(episode)
	if err != nil {
		return CausalAcquisitionReport{}, err
	}
	refs := make(map[string]struct{})
	acquiredCount := 0
	for index, use := range oracle.EvidenceUses {
		witnessAcquisition, err := privateWitnessAction(oracle, use.AcquisitionActionID)
		if err != nil {
			return CausalAcquisitionReport{}, err
		}
		witnessConsumer, err := privateWitnessAction(oracle, use.RequiredByActionID)
		if err != nil {
			return CausalAcquisitionReport{}, err
		}
		actualAcquisition, acquired, err := optionalActionForRequest(bindings, witnessAcquisition.Request)
		if err != nil {
			return CausalAcquisitionReport{}, fmt.Errorf("required evidence %d acquisition: %w", index+1, err)
		}
		actualConsumer, consumed, err := optionalActionForRequest(bindings, witnessConsumer.Request)
		if err != nil {
			return CausalAcquisitionReport{}, fmt.Errorf("required evidence %d consumer: %w", index+1, err)
		}
		if !acquired || !consumed {
			continue
		}
		if actualAcquisition.Sequence >= actualConsumer.Sequence {
			continue
		}
		observation, exists := observations[actualAcquisition.Action.ID]
		if !exists {
			continue
		}
		if err := observationContainsEvidence(observation, surfaceVersion, use.Evidence); err != nil {
			return CausalAcquisitionReport{}, fmt.Errorf("required evidence %d: %w", index+1, err)
		}
		refs[string(observation.ID)] = struct{}{}
		acquiredCount++
	}
	orderedRefs := make([]string, 0, len(refs))
	for ref := range refs {
		orderedRefs = append(orderedRefs, ref)
	}
	sort.Strings(orderedRefs)
	usesSHA, err := digestJSON(oracle.EvidenceUses)
	if err != nil {
		return CausalAcquisitionReport{}, fmt.Errorf("hash causal evidence uses: %w", err)
	}
	report := CausalAcquisitionReport{
		Schema: CausalAcquisitionReportSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256, SurfaceVersion: surfaceVersion,
		EvidenceUseSHA256: usesSHA, RequiredEvidence: len(oracle.RequiredEvidence),
		AcquiredEvidence: acquiredCount, AcquisitionTraceRefs: orderedRefs,
	}
	return report, report.Validate()
}

func acquisitionObservations(
	episode SealedEpisode,
) (map[cognition.ActionID]cognition.Observation, error) {
	observations := make(map[cognition.ActionID]cognition.Observation)
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind != TraceObservation {
			continue
		}
		observation := cognition.Observation{}
		if err := decodeTracePayload(entry.Payload, &observation, "causal acquisition observation"); err != nil {
			return nil, err
		}
		if err := observation.Validate(); err != nil || string(observation.ID) != entry.ID ||
			entry.Revision == nil || observation.Revision != *entry.Revision {
			return nil, fmt.Errorf("causal acquisition observation trace authority is invalid")
		}
		if observation.ActionID == "" {
			continue
		}
		if _, duplicate := observations[observation.ActionID]; duplicate {
			return nil, fmt.Errorf("causal acquisition action %q has duplicate observations", observation.ActionID)
		}
		observations[observation.ActionID] = observation
	}
	return observations, nil
}

func observationContainsEvidence(
	observation cognition.Observation,
	surfaceVersion string,
	evidence labyrinth.EvidenceIdentity,
) error {
	if surfaceVersion == "symbolic.v1" {
		var payload struct {
			Format        string                     `json:"format"`
			Predicates    []cognition.Predicate      `json:"predicates"`
			Records       []labyrinth.ObservedRecord `json:"records,omitempty"`
			GoalSatisfied bool                       `json:"goal_satisfied"`
		}
		if err := decodeStrictJSON([]byte(observation.Content), &payload, "symbolic acquisition result"); err != nil {
			return err
		}
		if payload.Format != "symbolic-observation.v1" {
			return fmt.Errorf("symbolic acquisition result has an invalid format")
		}
		matches := 0
		for _, record := range payload.Records {
			if string(record.ID) == evidence.ID && record.ContentSHA256 == evidence.SHA256 {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("exact evidence identity is absent from symbolic records")
		}
		return nil
	}
	var envelope struct {
		Surface          string          `json:"surface"`
		Operation        string          `json:"operation"`
		SymbolicState    json.RawMessage `json:"symbolic_state"`
		SurfaceAuthority string          `json:"surface_authority"`
		Result           json.RawMessage `json:"result"`
	}
	if err := decodeStrictJSON([]byte(observation.Content), &envelope, "surface acquisition result"); err != nil {
		return err
	}
	if envelope.Surface != surfaceVersion || !validDigest(envelope.SurfaceAuthority) ||
		len(envelope.SymbolicState) == 0 || len(envelope.Result) == 0 {
		return fmt.Errorf("surface acquisition result authority is invalid")
	}
	matches, conflicting, err := countEvidenceIdentities(envelope.Result, evidence)
	if err != nil {
		return err
	}
	if conflicting || matches != 1 {
		return fmt.Errorf("exact evidence identity is absent from bounded surface result")
	}
	return nil
}
