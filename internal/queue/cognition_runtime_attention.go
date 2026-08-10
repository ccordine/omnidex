package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func retainedCognitionAttention(
	set workingset.Snapshot,
	evidence []cognitionstate.EvidenceMaterial,
	current cognition.ObligationID,
) ([]cognition.AttentionRequest, error) {
	available := make(map[string]cognition.EvidenceRef, len(evidence))
	for _, material := range evidence {
		ref := taskstate.Ref{
			URI: cognitionEvidenceTaskRef(material.Ref), Version: fmt.Sprint(material.Ref.Revision.Number),
			Hash: material.Ref.SHA256, Relation: taskstate.RefEvidence,
		}
		available[taskstate.RefIdentity(ref)] = material.Ref
	}
	requests := make([]cognition.AttentionRequest, 0)
	for _, item := range set.Items {
		if item.State != workingset.ItemResident || item.Role != workingset.RoleEvidence ||
			item.Retention == workingset.RetentionPinned {
			continue
		}
		scope, applicable, err := retainedCognitionAttentionScope(item, set.Scope, current)
		if err != nil {
			return nil, err
		}
		if !applicable {
			continue
		}
		ref, exists := available[taskstate.RefIdentity(item.Ref)]
		if !exists || ref.SHA256 != item.Ref.Hash {
			return nil, fmt.Errorf(
				"%w: retained cognition evidence is unavailable: item=%q ref=%q version=%q hash=%q relation=%q memberships=%v current_obligation=%q",
				ErrCognitionConflict, item.ID, item.Ref.URI, item.Ref.Version, item.Ref.Hash,
				item.Ref.Relation, item.Memberships, current,
			)
		}
		requests = append(requests, cognition.AttentionRequest{
			Operation: cognition.AttentionRetain, TargetRef: ref,
			Scope:  scope,
			Reason: "Code preserved active scoped retention.",
		})
	}
	if len(requests) > cognitionstate.MaxAdvisoryRetains {
		return nil, fmt.Errorf("%w: retained cognition evidence exceeds the advisory cap", ErrCognitionConflict)
	}
	return requests, nil
}

func retainedCognitionAttentionScope(
	item workingset.Item,
	root workingset.Scope,
	current cognition.ObligationID,
) (cognition.AttentionScope, bool, error) {
	episode, err := cognitionstate.AttentionMembership(
		cognition.AttentionScopeEpisode, root, current, "",
	)
	if err != nil {
		return "", false, err
	}
	obligation, err := cognitionstate.AttentionMembership(
		cognition.AttentionScopeObligation, root, current, "",
	)
	if err != nil {
		return "", false, err
	}
	applicable := false
	for _, membership := range item.Memberships {
		if !cognitionstate.AttentionMembershipApplies(membership, root, current) {
			continue
		}
		applicable = true
		if membership == episode {
			return cognition.AttentionScopeEpisode, true, nil
		}
		if membership == obligation {
			continue
		}
		return "", false, fmt.Errorf("%w: applicable cognition attention membership is unregistered", ErrCognitionConflict)
	}
	if applicable {
		return cognition.AttentionScopeObligation, true, nil
	}
	return "", false, nil
}

func retainedCognitionAttentionNotOverridden(
	retained []cognition.AttentionRequest,
	requested []cognition.AttentionRequest,
) ([]cognition.AttentionRequest, error) {
	overridden := make(map[cognition.EvidenceRef]struct{}, len(requested))
	for _, request := range requested {
		overridden[request.TargetRef] = struct{}{}
	}
	result := make([]cognition.AttentionRequest, 0, len(retained))
	for _, request := range retained {
		if _, exists := overridden[request.TargetRef]; !exists {
			result = append(result, request)
		}
	}
	if len(result) > cognitionstate.MaxAdvisoryRetains {
		return nil, fmt.Errorf("%w: retained cognition evidence exceeds the fixed cap", ErrCognitionConflict)
	}
	return result, nil
}
