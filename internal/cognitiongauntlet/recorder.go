package cognitiongauntlet

import (
	"fmt"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

type EpisodeRecorder struct {
	mu       sync.Mutex
	template EpisodeManifest
	trace    []TraceEntry
	sealed   bool
}

func NewEpisodeRecorder(template EpisodeManifest) (*EpisodeRecorder, error) {
	if template.Trace != nil || template.TraceSHA256 != "" || template.FinalRevision.Number != 0 ||
		template.Outcome.Terminal || template.Outcome.GoalSatisfied || template.Outcome.PublicOutcome != "" ||
		!template.EpisodeStartedAt.IsZero() || !template.SealedAt.IsZero() {
		return nil, fmt.Errorf("cognition episode recorder template contains runtime outcome state")
	}
	if template.Schema != EpisodeManifestSchemaV2 ||
		!episodePattern.MatchString(string(template.EpisodeID)) ||
		!scenarioPattern.MatchString(string(template.Scenario.ID)) ||
		!validDigest(template.Scenario.SHA256) || !validDigest(template.PublicRunAuthoritySHA256) ||
		!validVariant(template.Variant) {
		return nil, fmt.Errorf("cognition episode recorder template identity is invalid")
	}
	if err := template.RatGeneration.Validate(); err != nil {
		return nil, err
	}
	if err := template.StationBudget.Validate(); err != nil {
		return nil, err
	}
	if template.StationBudget.MaxInputBytes > template.RatGeneration.Fixed.ContextCeilingBytes ||
		template.StationBudget.MaxInputTokens+template.StationBudget.MaxOutputTokens >
			template.RatGeneration.Fixed.Brain.NativeContextLimit {
		return nil, fmt.Errorf("cognition episode recorder station budget exceeds frozen authority")
	}
	return &EpisodeRecorder{template: template, trace: make([]TraceEntry, 0, 64)}, nil
}

func (recorder *EpisodeRecorder) Append(
	kind TraceKind,
	id string,
	revision *cognition.WorldRevision,
	payload taskstate.JSONObject,
) error {
	if recorder == nil {
		return fmt.Errorf("cognition episode recorder is nil")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.sealed {
		return fmt.Errorf("cognition episode recorder is sealed")
	}
	if len(recorder.trace) == maxEpisodeTraceEntries {
		return fmt.Errorf("cognition episode trace exceeds %d entries", maxEpisodeTraceEntries)
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	clonedPayload, err := taskstate.NewJSONObject(payload.Bytes())
	if err != nil {
		return err
	}
	entry := TraceEntry{
		Sequence: uint64(len(recorder.trace) + 1), Kind: kind, ID: id,
		Revision: cloneWorldRevision(revision), Payload: clonedPayload,
	}
	entry.PayloadSHA256, err = digestJSON(entry.Payload)
	if err != nil {
		return fmt.Errorf("hash cognition trace payload: %w", err)
	}
	if err := entry.Validate(entry.Sequence); err != nil {
		return err
	}
	recorder.trace = append(recorder.trace, entry)
	return nil
}

func (recorder *EpisodeRecorder) Seal(
	path string,
	startedAt time.Time,
	sealedAt time.Time,
	final cognition.WorldRevision,
	outcome Outcome,
	resources Resources,
	memory MemoryMetrics,
	planning PlanningMetrics,
	recovery RecoveryMetrics,
) (SealedEpisode, error) {
	if recorder == nil {
		return SealedEpisode{}, fmt.Errorf("cognition episode recorder is nil")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.sealed {
		return SealedEpisode{}, fmt.Errorf("cognition episode recorder is sealed")
	}
	manifest := recorder.template
	trace, err := cloneTrace(recorder.trace)
	if err != nil {
		return SealedEpisode{}, err
	}
	manifest.Trace = trace
	manifest.EpisodeStartedAt = startedAt.UTC()
	manifest.SealedAt = sealedAt.UTC()
	manifest.FinalRevision = final
	manifest.Outcome = outcome
	manifest.Resources = resources
	manifest.Memory = memory
	manifest.Planning = planning
	manifest.Recovery = recovery
	seal, err := SealEpisode(path, manifest)
	if err != nil {
		return SealedEpisode{}, err
	}
	recorder.sealed = true
	return seal, nil
}

func cloneWorldRevision(revision *cognition.WorldRevision) *cognition.WorldRevision {
	if revision == nil {
		return nil
	}
	cloned := *revision
	return &cloned
}

func cloneTrace(trace []TraceEntry) ([]TraceEntry, error) {
	cloned := make([]TraceEntry, len(trace))
	for index, entry := range trace {
		entry.Revision = cloneWorldRevision(entry.Revision)
		payload, err := taskstate.NewJSONObject(entry.Payload.Bytes())
		if err != nil {
			return nil, fmt.Errorf("clone cognition trace payload: %w", err)
		}
		entry.Payload = payload
		cloned[index] = entry
	}
	return cloned, nil
}
