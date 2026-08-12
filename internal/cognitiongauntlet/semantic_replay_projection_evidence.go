package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func verifySemanticSnapshotEvidenceRefs(
	projection contextbuilder.Projection,
	snapshotRefs []cognition.EvidenceRef,
	observations map[cognition.ObservationID]cognition.EvidenceRef,
	acceptedFacts map[string]queue.CognitionAcceptedFactMaterializationMember,
	current cognition.WorldRevision,
	graph cognition.ObligationGraphSnapshot,
) error {
	candidates := make([]cognition.EvidenceRef, 0, len(observations))
	for _, ref := range observations {
		candidates = append(candidates, ref)
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].Revision.Number != candidates[right].Revision.Number {
			return candidates[left].Revision.Number < candidates[right].Revision.Number
		}
		return candidates[left].ObservationID < candidates[right].ObservationID
	})
	expected := make([]cognition.EvidenceRef, 0)
	seen := make(map[cognition.EvidenceRef]struct{})
	for _, selected := range projection.Selected {
		switch selected.Role {
		case workingset.RoleEvidence:
			ref, err := queue.ResolveCognitionProjectionEvidenceRef(
				selected.Ref, candidates,
			)
			if err != nil {
				return fmt.Errorf("resolve semantic projected evidence: %w", err)
			}
			if _, duplicate := seen[ref]; !duplicate {
				seen[ref] = struct{}{}
				expected = append(expected, ref)
			}
		case workingset.RoleFact:
			member, exists := acceptedFacts[selected.Ref.URI]
			if !exists || verifySemanticAcceptedFactSelection(selected, member) != nil {
				return fmt.Errorf("semantic projected fact lacks exact code-owned materialization")
			}
			if err := verifySemanticAcceptedFactScope(
				member.Fact.ScopeObligationID, graph, current,
			); err != nil {
				return err
			}
			refs, err := queue.ResolveCognitionAcceptedFactProjection(
				selected.Ref, selected.SourceRefs, member,
			)
			if err != nil {
				return fmt.Errorf("resolve semantic projected fact: %w", err)
			}
			if err := verifySemanticAcceptedFactSources(refs, observations, current); err != nil {
				return err
			}
			for _, ref := range refs {
				if _, duplicate := seen[ref]; !duplicate {
					seen[ref] = struct{}{}
					expected = append(expected, ref)
				}
			}
		}
	}
	if !reflect.DeepEqual(snapshotRefs, expected) {
		return fmt.Errorf(
			"semantic runtime snapshot evidence differs from its exact Context Projection",
		)
	}
	return nil
}

func verifySemanticAcceptedFactSelection(
	selected contextbuilder.Selection,
	member queue.CognitionAcceptedFactMaterializationMember,
) error {
	if selected.Authority != taskstate.AuthorityCode ||
		selected.ContentSHA256 != selected.Ref.Hash ||
		selected.ContentSHA256 != digestExactBytes([]byte(member.Command.Content)) {
		return fmt.Errorf("semantic projected fact render or authority changed")
	}
	return nil
}

func verifySemanticAcceptedFactScope(
	scope cognition.ObligationID,
	graph cognition.ObligationGraphSnapshot,
	current cognition.WorldRevision,
) error {
	if graph.Validate() != nil || current.Validate() != nil || scope == "" {
		return fmt.Errorf("semantic projected fact scope authority is invalid")
	}
	byID := make(map[cognition.ObligationID]cognition.Obligation, len(graph.Obligations))
	var active cognition.Obligation
	activeFound := false
	for _, obligation := range graph.Obligations {
		byID[obligation.ID] = obligation
		if obligation.Status == cognition.ObligationActive {
			if activeFound {
				return fmt.Errorf("semantic projected fact graph has multiple active obligations")
			}
			active, activeFound = obligation, true
		}
	}
	if !activeFound {
		return fmt.Errorf("semantic projected fact graph lacks an active obligation")
	}
	seen := make(map[cognition.ObligationID]struct{})
	for cursor := active; ; {
		if cursor.ID == scope {
			return nil
		}
		if _, duplicate := seen[cursor.ID]; duplicate {
			break
		}
		seen[cursor.ID] = struct{}{}
		if cursor.ParentID == "" {
			break
		}
		parent, exists := byID[cursor.ParentID]
		if !exists {
			break
		}
		cursor = parent
	}
	return fmt.Errorf("semantic projected fact scope is not the active obligation or an ancestor")
}

func verifySemanticAcceptedFactSources(
	refs []cognition.EvidenceRef,
	observations map[cognition.ObservationID]cognition.EvidenceRef,
	current cognition.WorldRevision,
) error {
	for _, ref := range refs {
		observed, exists := observations[ref.ObservationID]
		if !exists || observed != ref || ref.Revision.EpisodeID != current.EpisodeID ||
			ref.Revision.Number >= current.Number {
			return fmt.Errorf("semantic projected fact cites unavailable prior evidence")
		}
	}
	return nil
}
