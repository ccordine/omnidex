package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const maxEpisodeTraceEntries = 1_000_000

func (manifest EpisodeManifest) validateSealed() error {
	if manifest.Schema != EpisodeManifestSchemaV1 ||
		!episodePattern.MatchString(string(manifest.EpisodeID)) ||
		!scenarioPattern.MatchString(string(manifest.Scenario.ID)) ||
		!validDigest(manifest.Scenario.SHA256) || !validDigest(manifest.PublicRunAuthoritySHA256) ||
		!validVariant(manifest.Variant) || !validDigest(manifest.TraceSHA256) {
		return fmt.Errorf("cognition episode manifest identity is invalid")
	}
	for label, value := range map[string]string{
		"runtime version":            manifest.RuntimeVersion,
		"ledger schema version":      manifest.LedgerSchemaVersion,
		"working-set policy version": manifest.WorkingSetPolicyVersion,
		"projection policy version":  manifest.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if manifest.OmnidexCommit != "" && !validCommitIdentity(manifest.OmnidexCommit) {
		return fmt.Errorf("cognition episode Omnidex commit must be an exact digest when present")
	}
	if err := manifest.RatGeneration.Validate(); err != nil {
		return fmt.Errorf("cognition episode rat generation: %w", err)
	}
	if err := manifest.Model.Validate(); err != nil {
		return err
	}
	if err := manifest.StationBudget.Validate(); err != nil {
		return err
	}
	brain := manifest.RatGeneration.Fixed.Brain
	if manifest.RuntimeVersion != manifest.RatGeneration.Runtime.Version ||
		manifest.Model.Name != brain.Model || manifest.Model.Digest != brain.Digest ||
		manifest.Model.Quantization != brain.Quantization ||
		manifest.Model.SamplingSHA256 != brain.SamplingSHA256 ||
		manifest.Model.ContextLimit != brain.NativeContextLimit ||
		manifest.Model.Backend != brain.Backend ||
		manifest.Model.BackendVersion != brain.BackendVersion ||
		manifest.Model.Hardware != brain.Hardware ||
		manifest.Model.HardwareAuthoritySource != brain.HardwareAuthoritySource {
		return fmt.Errorf("cognition episode changed its frozen rat generation authority")
	}
	if manifest.StationBudget.MaxInputBytes > manifest.RatGeneration.Fixed.ContextCeilingBytes ||
		manifest.StationBudget.MaxInputTokens+manifest.StationBudget.MaxOutputTokens > brain.NativeContextLimit {
		return fmt.Errorf("cognition episode station budget exceeds frozen context authority")
	}
	if manifest.FinalRevision.EpisodeID != manifest.EpisodeID || manifest.FinalRevision.Number == 0 ||
		!validDigest(manifest.FinalRevision.SHA256) {
		return fmt.Errorf("cognition episode final revision is invalid")
	}
	if !manifest.Outcome.Terminal && manifest.Outcome.GoalSatisfied {
		return fmt.Errorf("cognition episode cannot satisfy its goal before terminal state")
	}
	if err := requireExact(manifest.Outcome.PublicOutcome, "cognition public outcome", 4096); err != nil {
		return err
	}
	if manifest.Outcome.FailureCode != "" {
		if err := requireExact(manifest.Outcome.FailureCode, "cognition public failure code", 256); err != nil {
			return err
		}
	}
	if manifest.Trace == nil || len(manifest.Trace) == 0 || len(manifest.Trace) > maxEpisodeTraceEntries {
		return fmt.Errorf("cognition episode trace must be an explicit bounded nonempty array")
	}
	if manifest.Resources.PeakContextBytes > int64(manifest.RatGeneration.Fixed.ContextCeilingBytes) {
		return fmt.Errorf("cognition episode exceeded its frozen context ceiling")
	}
	var previousRevision uint64
	var actions, restarts, staleRejections, terminals int
	station := stationTraceState{}
	traceIDs := make(map[string]struct{}, len(manifest.Trace))
	for index, entry := range manifest.Trace {
		if err := entry.Validate(uint64(index + 1)); err != nil {
			return err
		}
		if entry.Revision != nil {
			if entry.Revision.EpisodeID != manifest.EpisodeID ||
				entry.Revision.Number < previousRevision ||
				entry.Revision.Number > manifest.FinalRevision.Number {
				return fmt.Errorf("cognition trace entry %d has inconsistent revision authority", index+1)
			}
			previousRevision = entry.Revision.Number
		}
		if _, duplicate := traceIDs[entry.ID]; duplicate {
			return fmt.Errorf("cognition trace entry %d duplicates identity %q", index+1, entry.ID)
		}
		traceIDs[entry.ID] = struct{}{}
		switch entry.Kind {
		case TraceProjection:
			if err := station.acceptProjection(entry); err != nil {
				return err
			}
		case TraceModelCall:
			if err := station.acceptModelCall(entry, manifest.StationBudget); err != nil {
				return err
			}
		case TraceAction:
			if _, err := decodeActionTrace(entry, cognition.EpisodeRef{ID: manifest.EpisodeID}); err != nil {
				return err
			}
			actions++
		case TraceRestart:
			restarts++
		case TraceStaleRejection:
			staleRejections++
		case TraceTerminal:
			terminals++
			if index != len(manifest.Trace)-1 || entry.Revision == nil ||
				*entry.Revision != manifest.FinalRevision {
				return fmt.Errorf("cognition terminal trace must be last and bind the final revision")
			}
		}
	}
	if terminals != 1 {
		return fmt.Errorf("cognition trace must contain one terminal event")
	}
	if err := station.validateResources(manifest.Resources); err != nil {
		return err
	}
	if actions != manifest.Resources.EnvironmentActions ||
		restarts != manifest.Recovery.Restarts || staleRejections != manifest.Recovery.StaleAttemptRejections ||
		manifest.Resources.ModelDecisions > manifest.Resources.ModelCalls {
		return fmt.Errorf("cognition trace counts do not match sealed resource and recovery metrics")
	}
	if manifest.Variant == VariantDeterministicOracle &&
		(manifest.Resources.ModelCalls != 0 || manifest.Resources.ModelDecisions != 0 ||
			manifest.Resources.InputTokens != 0 || manifest.Resources.OutputTokens != 0 ||
			manifest.Resources.ContextBytes != 0 || manifest.Resources.PeakContextBytes != 0) {
		return fmt.Errorf("deterministic oracle episode cannot claim model execution")
	}
	expectedTrace, err := digestJSON(manifest.Trace)
	if err != nil || expectedTrace != manifest.TraceSHA256 {
		return fmt.Errorf("cognition episode trace digest is inconsistent")
	}
	if err := manifest.Resources.Validate(); err != nil {
		return err
	}
	if err := manifest.Memory.Validate(); err != nil {
		return err
	}
	if err := manifest.Planning.Validate(); err != nil {
		return err
	}
	return manifest.Recovery.Validate()
}

func (record ModelRecord) Validate() error {
	for label, value := range map[string]string{
		"model name": record.Name, "quantization": record.Quantization,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if !validDigest(record.Digest) || !validDigest(record.SamplingSHA256) ||
		record.ContextLimit <= 0 || record.ContextLimit > 10_000_000 {
		return fmt.Errorf("cognition episode model digest or context limit is invalid")
	}
	for label, value := range map[string]string{
		"hardware": record.Hardware, "hardware authority source": record.HardwareAuthoritySource,
		"backend":         record.Backend,
		"backend version": record.BackendVersion,
	} {
		if err := requireExact(value, label, 512); err != nil {
			return err
		}
	}
	return nil
}

func (entry TraceEntry) Validate(expectedSequence uint64) error {
	if entry.Sequence != expectedSequence || !validTraceKind(entry.Kind) ||
		requireExact(entry.ID, "trace entry ID", 512) != nil || !validDigest(entry.PayloadSHA256) {
		return fmt.Errorf("cognition trace entry %d identity is invalid", expectedSequence)
	}
	if err := entry.Payload.Validate(); err != nil {
		return fmt.Errorf("cognition trace entry %d payload: %w", expectedSequence, err)
	}
	payloadDigest, err := digestJSON(entry.Payload)
	if err != nil || payloadDigest != entry.PayloadSHA256 {
		return fmt.Errorf("cognition trace entry %d payload digest is inconsistent", expectedSequence)
	}
	if entry.Revision != nil && (entry.Revision.Number == 0 || !validDigest(entry.Revision.SHA256)) {
		return fmt.Errorf("cognition trace entry %d revision is invalid", expectedSequence)
	}
	return nil
}

func (resources Resources) Validate() error {
	if resources.ModelCalls < 0 || resources.ModelDecisions < 0 || resources.EnvironmentActions < 0 ||
		resources.LowLevelTransitions < 0 || resources.ToolOperations < 0 ||
		resources.SearchOperations < 0 || resources.ReadOperations < 0 ||
		resources.SearchOperations+resources.ReadOperations > resources.ToolOperations ||
		resources.InputTokens < 0 || resources.OutputTokens < 0 ||
		resources.ContextBytes < 0 || resources.OutputBytes < 0 ||
		resources.PeakContextBytes < 0 || resources.PeakWorkingSetBytes < 0 ||
		resources.ModelMilliseconds < 0 || resources.WallMilliseconds < 0 {
		return fmt.Errorf("cognition episode resources cannot be negative")
	}
	return nil
}

func (metrics MemoryMetrics) Validate() error {
	if metrics.CriticalEvidenceAcquired < 0 || metrics.CriticalEvidenceAtUse < 0 ||
		metrics.CriticalEvidenceAtUse > metrics.CriticalEvidenceAcquired ||
		metrics.ProjectionMisses < 0 || metrics.StaleResidentBytes < 0 ||
		metrics.IrrelevantResidentBytes < 0 || metrics.ReleaseLatencyActions < 0 ||
		metrics.Reacquisitions < 0 || metrics.Thrashes < 0 {
		return fmt.Errorf("cognition episode memory metrics are invalid")
	}
	return nil
}

func (metrics PlanningMetrics) Validate() error {
	if metrics.ObligationsCreated < 0 || metrics.ObligationsCompleted < 0 ||
		metrics.ObligationsCompleted > metrics.ObligationsCreated || metrics.PlanGenerations < 0 ||
		metrics.UnnecessarySubgoals < 0 || metrics.DeadEndRevisits < 0 ||
		metrics.UnsupportedActions < 0 || metrics.InvalidActions < 0 || metrics.Backtracks < 0 {
		return fmt.Errorf("cognition episode planning metrics are invalid")
	}
	return nil
}

func (metrics RecoveryMetrics) Validate() error {
	if metrics.Restarts < 0 || metrics.RestorationMismatches < 0 ||
		metrics.DuplicateSuppressions < 0 || metrics.StaleAttemptRejections < 0 ||
		metrics.ProjectionMismatches < 0 {
		return fmt.Errorf("cognition episode recovery metrics are invalid")
	}
	return nil
}

func validTraceKind(kind TraceKind) bool {
	switch kind {
	case TraceModelCall, TraceProjection, TraceObservation, TraceAction, TraceLedger,
		TraceWorkingSet, TraceObligation, TraceFailure, TraceRestart, TraceLease,
		TraceStaleRejection, TraceTerminal:
		return true
	default:
		return false
	}
}
