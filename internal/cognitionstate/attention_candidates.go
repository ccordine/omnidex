package cognitionstate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type attentionCandidate struct {
	key         string
	ref         taskstate.Ref
	role        workingset.Role
	authority   taskstate.Authority
	content     string
	sourceRefs  []taskstate.Ref
	priority    int
	mandatory   bool
	pinned      bool
	advisory    bool
	memberships []workingset.Membership
}

func buildMandatoryCandidates(input ReconciliationInput) ([]attentionCandidate, map[cognition.EvidenceRef]EvidenceMaterial, error) {
	evidence, err := validateReconciliationInput(input)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]attentionCandidate, 0)
	goal, err := valueCandidate("goal", input.State.SHA256(), input.State.Goal(), workingset.RoleGoal, 100)
	if err != nil {
		return nil, nil, err
	}
	goal.ref.URI = "cognition:goal/" + input.State.SHA256()
	goal.ref.Relation = taskstate.RefConcerns
	candidates = append(candidates, goal)
	current := input.State.Obligation()
	relevantScopes, err := relevantObligationScopes(input.ObligationGraph, current)
	if err != nil {
		return nil, nil, err
	}
	currentCandidate, err := obligationCandidate(current, workingset.RoleTask, 100)
	if err != nil {
		return nil, nil, err
	}
	candidates = append(candidates, currentCandidate)
	revision, err := valueCandidate(
		"revision", strconv.FormatUint(input.State.Revision().Number, 10),
		input.State.Revision(), workingset.RoleInvariant, 100,
	)
	if err != nil {
		return nil, nil, err
	}
	revision.ref = revisionLedgerRef(input.State.Revision())
	candidates = append(candidates, revision)

	for _, entry := range input.Ledger.Entries {
		if entry.Status == taskstate.EntryActive && entry.Kind == taskstate.EntryConstraint &&
			entryAppliesToCurrentObligation(entry, relevantScopes) {
			candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, entry, workingset.RoleConstraint, 95))
		}
		if entry.Status != taskstate.EntryActive || !entryScopedToCurrentObligation(entry, relevantScopes) {
			continue
		}
		switch entry.Kind {
		case taskstate.EntryFact:
			eligible, eligibilityErr := acceptedFactEligibleAfterSourceRevision(entry, input.State.Revision())
			if eligibilityErr != nil {
				return nil, nil, eligibilityErr
			}
			if !eligible {
				continue
			}
			candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, entry, workingset.RoleFact, 94))
		case taskstate.EntryAcceptedDecision:
			candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, entry, workingset.RoleDecision, 93))
		case taskstate.EntryQuestion:
			candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, entry, workingset.RoleQuestion, 91))
		case taskstate.EntryHypothesis:
			candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, entry, workingset.RoleHypothesis, 90))
		}
	}
	if failure, exists := latestActiveFailure(input.Ledger.Entries, relevantScopes); exists {
		candidates = append(candidates, ledgerEntryCandidate(input.Ledger.ID, failure, workingset.RoleFailure, 98))
	}

	causal := make(map[cognition.EvidenceRef]struct{}, len(current.SupportingRefs))
	for _, ref := range current.SupportingRefs {
		causal[ref] = struct{}{}
	}
	causalRefs := make([]cognition.EvidenceRef, 0, len(causal))
	for ref := range causal {
		causalRefs = append(causalRefs, ref)
	}
	sort.Slice(causalRefs, func(left, right int) bool {
		return evidenceRefKey(causalRefs[left]) < evidenceRefKey(causalRefs[right])
	})
	for _, ref := range causalRefs {
		material, exists := evidence[ref]
		if !exists {
			return nil, nil, fmt.Errorf("%w: %s", ErrMissingMaterial, ref.ObservationID)
		}
		candidates, err = mergeAttentionCandidate(candidates, evidenceCandidate(material, true))
		if err != nil {
			return nil, nil, err
		}
	}
	for _, material := range input.Evidence {
		if material.Ref.Revision != input.State.Revision() {
			continue
		}
		membership, membershipErr := AttentionMembership(
			cognition.AttentionScopeDecision, input.WorkingSet.Scope,
			current.ID, input.State.Revision().SHA256,
		)
		if membershipErr != nil {
			return nil, nil, membershipErr
		}
		candidate := evidenceCandidate(material, true)
		candidate.pinned = false
		candidate.memberships = []workingset.Membership{membership}
		candidates, err = mergeAttentionCandidate(candidates, candidate)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := validateCandidateSet(candidates); err != nil {
		return nil, nil, err
	}
	return candidates, evidence, nil
}

func relevantObligationScopes(
	graph cognition.ObligationGraphSnapshot,
	current cognition.Obligation,
) (map[taskstate.NodeID]struct{}, error) {
	byID := make(map[cognition.ObligationID]cognition.Obligation, len(graph.Obligations))
	for _, obligation := range graph.Obligations {
		byID[obligation.ID] = obligation
	}
	result := make(map[taskstate.NodeID]struct{})
	for candidate := current; ; {
		result[taskstate.NodeID(candidate.ID)] = struct{}{}
		if candidate.ParentID == "" {
			break
		}
		parent, exists := byID[candidate.ParentID]
		if !exists {
			return nil, fmt.Errorf("%w: current obligation ancestor %q is missing", ErrInvalidReconciliation, candidate.ParentID)
		}
		candidate = parent
	}
	return result, nil
}

func entryAppliesToCurrentObligation(
	entry taskstate.Entry,
	relevant map[taskstate.NodeID]struct{},
) bool {
	if entry.ScopeNodeID == "" {
		return true
	}
	_, exists := relevant[entry.ScopeNodeID]
	return exists
}

func entryScopedToCurrentObligation(
	entry taskstate.Entry,
	relevant map[taskstate.NodeID]struct{},
) bool {
	if entry.ScopeNodeID == "" {
		return false
	}
	_, exists := relevant[entry.ScopeNodeID]
	return exists
}

func validateReconciliationInput(input ReconciliationInput) (map[cognition.EvidenceRef]EvidenceMaterial, error) {
	if err := input.State.Validate(); err != nil {
		return nil, err
	}
	if err := input.ObligationGraph.Validate(); err != nil {
		return nil, fmt.Errorf("%w: obligation graph: %v", ErrInvalidReconciliation, err)
	}
	if err := taskstate.ValidateMaterializedState(input.Ledger); err != nil {
		return nil, fmt.Errorf("%w: ledger: %v", ErrInvalidReconciliation, err)
	}
	if err := workingset.ValidateSnapshot(input.WorkingSet); err != nil {
		return nil, fmt.Errorf("%w: working set: %v", ErrInvalidReconciliation, err)
	}
	actor, owner := input.State.Attempt(), input.WorkingSet.Owner
	if input.Ledger.Status != taskstate.LedgerActive || owner.LedgerID != input.Ledger.ID ||
		owner.JobID != input.Ledger.Owner.JobID || actor.JobID != owner.JobID || actor.Generation != owner.Generation {
		return nil, fmt.Errorf("%w: runtime, ledger, graph, and working-set authorities differ", ErrInvalidReconciliation)
	}
	current, exists := obligationByID(input.ObligationGraph, input.State.Obligation().ID)
	if !exists || !sameJSON(current, input.State.Obligation()) {
		return nil, fmt.Errorf("%w: current obligation is absent from the graph", ErrInvalidReconciliation)
	}
	root, exists := obligationByID(input.ObligationGraph, input.ObligationGraph.RootID)
	if !exists || !sameJSON(root.Desired, input.State.Goal()) {
		return nil, fmt.Errorf("%w: graph root does not bind the runtime goal", ErrInvalidReconciliation)
	}
	materials := make(map[cognition.EvidenceRef]EvidenceMaterial, len(input.Evidence))
	for index, material := range input.Evidence {
		if err := material.Ref.Validate(); err != nil || material.Content == "" || !utf8.ValidString(material.Content) ||
			strings.ContainsRune(material.Content, 0) || mappingTextDigest(material.Content) != material.Ref.SHA256 {
			return nil, fmt.Errorf("%w: evidence material %d is invalid", ErrInvalidReconciliation, index)
		}
		if _, duplicate := materials[material.Ref]; duplicate {
			return nil, fmt.Errorf("%w: evidence material %d is duplicated", ErrInvalidReconciliation, index)
		}
		materials[material.Ref] = material
	}
	return materials, nil
}

func valueCandidate(key, version string, value any, role workingset.Role, priority int) (attentionCandidate, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return attentionCandidate{}, fmt.Errorf("%w: encode %s: %v", ErrInvalidReconciliation, key, err)
	}
	content := string(raw)
	return attentionCandidate{
		key: key, ref: taskstate.Ref{Version: version, Hash: mappingTextDigest(content), Relation: taskstate.RefSource},
		role: role, authority: taskstate.AuthorityCode, content: content,
		sourceRefs: []taskstate.Ref{}, priority: priority, mandatory: true, pinned: true,
	}, nil
}

func obligationCandidate(obligation cognition.Obligation, role workingset.Role, priority int) (attentionCandidate, error) {
	candidate, err := valueCandidate(
		"obligation:"+string(obligation.ID),
		strconv.FormatUint(obligation.CreatedGeneration, 10), obligation, role, priority,
	)
	if err != nil {
		return attentionCandidate{}, err
	}
	// An obligation's lifecycle, dependencies, support, and completion can all
	// change within one plan generation. Its immutable reference version must
	// therefore bind the exact rendered value rather than the creation epoch.
	candidate.ref.Version = candidate.ref.Hash
	candidate.ref.URI = "cognition:obligation/" + string(obligation.ID)
	return candidate, nil
}

func ledgerEntryCandidate(ledgerID taskstate.LedgerID, entry taskstate.Entry, role workingset.Role, priority int) attentionCandidate {
	return attentionCandidate{
		key: "entry:" + string(entry.ID), ref: taskstate.Ref{
			URI:     "task:ledger/" + string(ledgerID) + "/entry/" + string(entry.ID),
			Version: strconv.FormatUint(entry.UpdatedVersion, 10), Hash: entry.ContentSHA256, Relation: taskstate.RefSource,
		},
		role: role, authority: entry.Authority, content: entry.Content,
		sourceRefs: append([]taskstate.Ref{}, entry.Refs...), priority: priority, mandatory: true, pinned: true,
	}
}
