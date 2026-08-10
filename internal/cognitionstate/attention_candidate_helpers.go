package cognitionstate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func evidenceCandidate(material EvidenceMaterial, mandatory bool) attentionCandidate {
	priority := 60
	if mandatory {
		priority = 92
	}
	return attentionCandidate{
		key: "evidence:" + evidenceRefKey(material.Ref), ref: evidenceLedgerRef(material.Ref),
		role: workingset.RoleEvidence, authority: taskstate.AuthorityToolEvidence,
		content: material.Content, sourceRefs: []taskstate.Ref{},
		priority: priority, mandatory: mandatory, pinned: mandatory,
	}
}

func mergeAttentionCandidate(
	candidates []attentionCandidate,
	candidate attentionCandidate,
) ([]attentionCandidate, error) {
	identity := taskstate.RefIdentity(candidate.ref)
	for index := range candidates {
		current := &candidates[index]
		if taskstate.RefIdentity(current.ref) != identity {
			continue
		}
		if current.ref.Hash != candidate.ref.Hash || current.role != candidate.role ||
			current.authority != candidate.authority || current.content != candidate.content ||
			!sameJSON(current.sourceRefs, candidate.sourceRefs) {
			return nil, fmt.Errorf("%w: candidate reference %q has conflicting material", ErrInvalidReconciliation, candidate.ref.URI)
		}
		current.mandatory = current.mandatory || candidate.mandatory
		current.pinned = current.pinned || candidate.pinned
		if candidate.priority > current.priority {
			current.priority = candidate.priority
		}
		for _, membership := range candidate.memberships {
			if !candidateHasMembership(*current, membership) {
				current.memberships = append(current.memberships, membership)
			}
		}
		return candidates, nil
	}
	return append(candidates, candidate), nil
}

func candidateHasMembership(candidate attentionCandidate, membership workingset.Membership) bool {
	for _, current := range candidate.memberships {
		if current == membership {
			return true
		}
	}
	return false
}

func latestActiveFailure(
	entries []taskstate.Entry,
	relevant map[taskstate.NodeID]struct{},
) (taskstate.Entry, bool) {
	var latest taskstate.Entry
	found := false
	for _, entry := range entries {
		if entry.Status != taskstate.EntryActive || entry.Kind != taskstate.EntryFailure ||
			!entryAppliesToCurrentObligation(entry, relevant) {
			continue
		}
		if !found || entry.UpdatedVersion > latest.UpdatedVersion ||
			entry.UpdatedVersion == latest.UpdatedVersion && entry.ID > latest.ID {
			latest, found = entry, true
		}
	}
	return latest, found
}

func obligationByID(snapshot cognition.ObligationGraphSnapshot, id cognition.ObligationID) (cognition.Obligation, bool) {
	for _, obligation := range snapshot.Obligations {
		if obligation.ID == id {
			return obligation.Clone(), true
		}
	}
	return cognition.Obligation{}, false
}

func sameJSON(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func evidenceRefKey(ref cognition.EvidenceRef) string {
	return string(ref.ObservationID) + "\x00" + string(ref.Revision.EpisodeID) + "\x00" +
		strconv.FormatUint(ref.Revision.Number, 10) + "\x00" + ref.SHA256
}

func validateCandidateSet(candidates []attentionCandidate) error {
	if len(candidates) == 0 || len(candidates) > MaxContextItems {
		return fmt.Errorf("%w: required context has %d items", ErrReconciliationCapacity, len(candidates))
	}
	bytes := 0
	seenRefs := make(map[string]string, len(candidates))
	seenRoles := make(map[workingset.Role]int)
	for index, candidate := range candidates {
		if err := taskstate.ValidateRef(candidate.ref); err != nil || candidate.content == "" ||
			candidate.priority < 1 || candidate.priority > 100 {
			return fmt.Errorf("%w: candidate %d is invalid", ErrInvalidReconciliation, index)
		}
		identity := taskstate.RefIdentity(candidate.ref)
		if hash, duplicate := seenRefs[identity]; duplicate {
			return fmt.Errorf("%w: candidate reference repeats with hashes %s and %s", ErrInvalidReconciliation, hash, candidate.ref.Hash)
		}
		seenRefs[identity] = candidate.ref.Hash
		seenRoles[candidate.role]++
		bytes += len(candidate.content)
	}
	if seenRoles[workingset.RoleGoal] != 1 || seenRoles[workingset.RoleTask] != 1 ||
		seenRoles[workingset.RoleInvariant] != 1 || bytes > MaxContextMaterialBytes {
		return fmt.Errorf("%w: required context identity or bytes exceed the fixed policy", ErrReconciliationCapacity)
	}
	return nil
}

func sortCandidates(candidates []attentionCandidate) {
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].mandatory != candidates[right].mandatory {
			return candidates[left].mandatory
		}
		if candidates[left].role != candidates[right].role {
			return candidates[left].role < candidates[right].role
		}
		return taskstate.RefIdentity(candidates[left].ref) < taskstate.RefIdentity(candidates[right].ref)
	})
}

func managedAttentionRef(ref taskstate.Ref, ledgerID taskstate.LedgerID) bool {
	return strings.HasPrefix(ref.URI, "cognition:") ||
		strings.HasPrefix(ref.URI, "task:ledger/"+string(ledgerID)+"/")
}
