package cognition

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	ObligationIdentitySchemaV1        = "omnidex.cognition-obligation-identity.v1"
	ObligationMaterializationSchemaV1 = "omnidex.cognition-obligation-materialization.v1"
)

func cognitionValueSHA256(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode cognition identity: %w", err)
	}
	return contentSHA256(string(raw)), nil
}

func DeriveObligationID(
	episodeID EpisodeID,
	generation uint64,
	parentID ObligationID,
	desired GoalExpression,
	check CompletionCheckRef,
) (ObligationID, error) {
	if err := (EpisodeRef{ID: episodeID}).Validate(); err != nil {
		return "", fmt.Errorf("%w: episode: %v", ErrInvalidObligationIdentity, err)
	}
	if generation == 0 {
		return "", fmt.Errorf("%w: generation must be positive", ErrInvalidObligationIdentity)
	}
	if parentID != "" {
		if err := validateIdentity(string(parentID), "obligation parent ID"); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidObligationIdentity, err)
		}
	}
	goal, err := canonicalGoal(desired)
	if err != nil {
		return "", fmt.Errorf("%w: desired goal: %v", ErrInvalidObligationIdentity, err)
	}
	if err := check.Validate(); err != nil {
		return "", fmt.Errorf("%w: completion check: %v", ErrInvalidObligationIdentity, err)
	}
	digest, err := cognitionValueSHA256(struct {
		Schema     string
		EpisodeID  EpisodeID
		Generation string
		ParentID   ObligationID
		Desired    GoalExpression
		Check      CompletionCheckRef
	}{ObligationIdentitySchemaV1, episodeID, strconv.FormatUint(generation, 10), parentID, goal, check})
	if err != nil {
		return "", err
	}
	return ObligationID("cognition_obligation_" + digest), nil
}

func obligationMaterializationSHA256(
	materialization ObligationMaterialization,
) (string, error) {
	return cognitionValueSHA256(struct {
		Schema, SourceSnapshotSHA256, SourceDecisionSHA256, SourceProposalSHA256 string
		ProposalIndex                                                            int
		EpisodeID                                                                EpisodeID
		Generation                                                               uint64
		ExpectedGraphSHA256                                                      string
		ActiveObligationID                                                       ObligationID
		CompletionAuthority                                                      CompletionAuthority
		Spec                                                                     ObligationSpec
		ResultGraphSHA256                                                        string
	}{
		materialization.Schema, materialization.SourceSnapshotSHA256,
		materialization.SourceDecisionSHA256, materialization.SourceProposalSHA256,
		materialization.ProposalIndex,
		materialization.EpisodeID, materialization.Generation,
		materialization.ExpectedGraphSHA256, materialization.ActiveObligationID,
		materialization.CompletionAuthority, materialization.Spec,
		materialization.ResultGraphSHA256,
	})
}
