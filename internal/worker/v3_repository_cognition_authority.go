package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
)

const (
	repositoryCognitionEpisodeSchemaV1 = "omnidex.repository-cognition-episode.v1"
)

func repositoryCognitionNeedAuthority(
	decision assemblyline.RepositoryRetrievalDecision,
) (cognitionenv.NeedAuthority, error) {
	// The accepted query is already the semantic station's bounded relevant
	// authority. Revalidating it against itself preserves that contract without
	// carrying the broader conversation into the cognition episode.
	input := assemblyline.RepositoryRetrievalInput{ResearchNeed: decision.QueryQuote}
	if err := decision.ValidateFor(input); err != nil {
		return cognitionenv.NeedAuthority{}, fmt.Errorf(
			"repository cognition retrieval authority: %w", err,
		)
	}
	return cognitionenv.NewNeedAuthority(decision.QueryQuote)
}

func repositoryCognitionBrain(runtime *nativeRuntimeV3) (cognitionpolicy.BrainRef, error) {
	if runtime == nil || runtime.svc == nil || runtime.claim == nil {
		return cognitionpolicy.BrainRef{}, fmt.Errorf("repository cognition requires a claimed worker runtime")
	}
	brain := runtime.svc.cognitionBrain
	if err := brain.Validate(); err != nil {
		return cognitionpolicy.BrainRef{}, fmt.Errorf("repository cognition brain: %w", err)
	}
	resolved := metadataModel(runtime.claim.Job, "model_analyze", runtime.routing.Analyze)
	if resolved == "" {
		return cognitionpolicy.BrainRef{}, fmt.Errorf("repository cognition requires an exact job-local analyze route")
	}
	if resolved != brain.Model {
		return cognitionpolicy.BrainRef{}, fmt.Errorf(
			"repository cognition brain %q differs from job-local analyze route %q",
			brain.Model, resolved,
		)
	}
	return brain, nil
}

func repositoryCognitionBudget(
	brain cognitionpolicy.BrainRef,
	catalog cognition.ActionCatalog,
) (cognition.RuntimeBudget, int, error) {
	if err := brain.Validate(); err != nil {
		return cognition.RuntimeBudget{}, 0, err
	}
	if err := catalog.Validate(); err != nil {
		return cognition.RuntimeBudget{}, 0, err
	}
	outputBytes := brain.Sampling.MaxOutputTokens * 4
	if outputBytes > cognition.MaxPolicyOutputBytes {
		outputBytes = cognition.MaxPolicyOutputBytes
	}
	maxArguments := 0
	for _, schema := range catalog.Schemas {
		if len(schema.Parameters) > maxArguments {
			maxArguments = len(schema.Parameters)
		}
	}
	policyCalls := uint32(len(catalog.Schemas))
	runCycles := policyCalls + 1
	budget := cognition.RuntimeBudget{
		RemainingPolicyCalls:   policyCalls,
		MaxInputBytes:          brain.ContextCeilingBytes,
		MaxInputTokens:         (brain.ContextCeilingBytes + 3) / 4,
		MaxOutputBytes:         outputBytes,
		MaxOutputTokens:        brain.Sampling.MaxOutputTokens,
		MaxEvidenceRefs:        1 + (2 * len(catalog.Schemas)),
		MaxActionArguments:     maxArguments,
		MaxLedgerProposals:     0,
		MaxAttentionRequests:   0,
		MaxExpectedEffectBytes: 512,
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(brain, budget); err != nil {
		return cognition.RuntimeBudget{}, 0, err
	}
	return budget, int(runCycles), nil
}

func repositoryCognitionEpisodeRef(
	authority model.StepAttemptAuthority,
	scenario cognition.ScenarioRef,
) (cognition.EpisodeRef, error) {
	if authority.JobID < 1 || authority.Generation < 1 || authority.StepID < 1 ||
		authority.Attempt < 1 || strings.TrimSpace(authority.WorkerID) == "" {
		return cognition.EpisodeRef{}, fmt.Errorf("repository cognition requires exact step-attempt authority")
	}
	if err := scenario.Validate(); err != nil {
		return cognition.EpisodeRef{}, err
	}
	raw, err := json.Marshal(struct {
		Schema     string
		JobID      int64
		Generation int64
		StepID     int64
		Scenario   cognition.ScenarioRef
	}{repositoryCognitionEpisodeSchemaV1, authority.JobID, authority.Generation, authority.StepID, scenario})
	if err != nil {
		return cognition.EpisodeRef{}, err
	}
	digest := sha256.Sum256(raw)
	return cognition.NewEpisodeRef(cognition.EpisodeID(
		"repository-cognition-" + hex.EncodeToString(digest[:]),
	))
}
