package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const ResumeEpisodeSemanticsSchemaV2 = "omnidex.resume-episode-semantics.v2"

// ResumeEpisodeSemantics deliberately excludes attempt-bound identities. It
// binds the exact model-visible projection sequence and world-changing action
// sequence so interrupted runs can be compared with their uninterrupted pair.
type ResumeEpisodeSemantics struct {
	Schema                   string                  `json:"schema"`
	ProjectionSequenceSHA256 string                  `json:"projection_sequence_sha256"`
	LogicalProjectionSHA256  string                  `json:"logical_projection_sha256"`
	ActionSequenceSHA256     string                  `json:"action_sequence_sha256"`
	FinalRevision            cognition.WorldRevision `json:"final_revision"`
	Outcome                  Outcome                 `json:"outcome"`
	ModelCalls               int                     `json:"model_calls"`
	ModelDecisions           int                     `json:"model_decisions"`
	EnvironmentActions       int                     `json:"environment_actions"`
	ProjectionCount          int                     `json:"projection_count"`
	LogicalProjectionCount   int                     `json:"logical_projection_count"`
}

type resumeProjectionSemantic struct {
	RenderedSHA256 string `json:"rendered_sha256"`
	RenderedBytes  int64  `json:"rendered_bytes"`
}

type resumeActionSemantic struct {
	Request          cognition.ActionRequest      `json:"request"`
	ExpectedRevision cognition.WorldRevision      `json:"expected_revision"`
	Result           *resumeTransitionSemantic    `json:"result,omitempty"`
	Failure          *resumeActionFailureSemantic `json:"failure,omitempty"`
}

type resumeTransitionSemantic struct {
	Current       cognition.WorldRevision `json:"current"`
	Cost          int                     `json:"cost"`
	Terminal      bool                    `json:"terminal"`
	PublicOutcome string                  `json:"public_outcome"`
}

type resumeActionFailureSemantic struct {
	Code          cognition.ActionFailureCode `json:"code"`
	Revision      cognition.WorldRevision     `json:"revision"`
	PublicMessage string                      `json:"public_message"`
}

func DeriveResumeEpisodeSemantics(episode SealedEpisode) (ResumeEpisodeSemantics, error) {
	if err := episode.Validate(); err != nil {
		return ResumeEpisodeSemantics{}, err
	}
	projections := make([]resumeProjectionSemantic, 0, episode.Manifest.Resources.ModelCalls)
	actions := make([]resumeActionSemantic, 0, episode.Manifest.Resources.EnvironmentActions)
	for index, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceProjection:
			var projection ProjectionTrace
			if err := decodeTracePayload(entry.Payload, &projection, "Resume projection"); err != nil {
				return ResumeEpisodeSemantics{}, fmt.Errorf("trace %d: %w", index+1, err)
			}
			if err := projection.Validate(); err != nil {
				return ResumeEpisodeSemantics{}, err
			}
			projections = append(projections, resumeProjectionSemantic{
				RenderedSHA256: projection.ProjectionSHA256,
				RenderedBytes:  projection.RenderedBytes,
			})
		case TraceAction:
			var action ActionTrace
			if err := decodeTracePayload(entry.Payload, &action, "Resume action"); err != nil {
				return ResumeEpisodeSemantics{}, fmt.Errorf("trace %d: %w", index+1, err)
			}
			actions = append(actions, normalizeResumeAction(action))
		}
	}
	projectionSHA, err := digestJSON(projections)
	if err != nil {
		return ResumeEpisodeSemantics{}, err
	}
	actionSHA, err := digestJSON(actions)
	if err != nil {
		return ResumeEpisodeSemantics{}, err
	}
	logical := dedupeResumeProjections(projections)
	logicalSHA, err := digestJSON(logical)
	if err != nil {
		return ResumeEpisodeSemantics{}, err
	}
	result := ResumeEpisodeSemantics{
		Schema:                   ResumeEpisodeSemanticsSchemaV2,
		ProjectionSequenceSHA256: projectionSHA, ActionSequenceSHA256: actionSHA,
		LogicalProjectionSHA256: logicalSHA,
		FinalRevision:           episode.Manifest.FinalRevision, Outcome: episode.Manifest.Outcome,
		ModelCalls:         episode.Manifest.Resources.ModelCalls,
		ModelDecisions:     episode.Manifest.Resources.ModelDecisions,
		EnvironmentActions: episode.Manifest.Resources.EnvironmentActions,
		ProjectionCount:    len(projections), LogicalProjectionCount: len(logical),
	}
	return result, result.Validate()
}

func dedupeResumeProjections(
	values []resumeProjectionSemantic,
) []resumeProjectionSemantic {
	result := make([]resumeProjectionSemantic, 0, len(values))
	for _, value := range values {
		if len(result) > 0 && result[len(result)-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func normalizeResumeAction(action ActionTrace) resumeActionSemantic {
	result := resumeActionSemantic{
		Request: action.Action.Request, ExpectedRevision: action.ExpectedRevision,
	}
	if action.Transition != nil {
		result.Result = &resumeTransitionSemantic{
			Current: action.Transition.Current, Cost: action.Transition.Cost,
			Terminal: action.Transition.Terminal, PublicOutcome: action.Transition.PublicOutcome,
		}
	}
	if action.Failure != nil {
		result.Failure = &resumeActionFailureSemantic{
			Code: action.Failure.Code, Revision: action.Failure.Revision,
			PublicMessage: action.Failure.PublicMessage,
		}
	}
	return result
}

func (semantics ResumeEpisodeSemantics) Validate() error {
	if semantics.Schema != ResumeEpisodeSemanticsSchemaV2 ||
		!validDigest(semantics.ProjectionSequenceSHA256) ||
		!validDigest(semantics.LogicalProjectionSHA256) ||
		!validDigest(semantics.ActionSequenceSHA256) || semantics.FinalRevision.Validate() != nil ||
		semantics.ModelCalls < 0 || semantics.ModelDecisions < 0 ||
		semantics.EnvironmentActions < 0 || semantics.ModelDecisions > semantics.ModelCalls ||
		semantics.ProjectionCount < semantics.ModelCalls || semantics.LogicalProjectionCount < 0 ||
		semantics.LogicalProjectionCount > semantics.ProjectionCount {
		return fmt.Errorf("Resume episode semantics authority is invalid")
	}
	return nil
}
