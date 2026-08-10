package labyrinth

import (
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

type solverEvidence map[EntityID]string

type solverEvidenceIdentity struct {
	ID     EntityID `json:"id"`
	SHA256 string   `json:"sha256"`
}

func (evidence solverEvidence) clone() solverEvidence {
	cloned := make(solverEvidence, len(evidence))
	for id, digest := range evidence {
		cloned[id] = digest
	}
	return cloned
}

func (evidence solverEvidence) sorted() []solverEvidenceIdentity {
	values := make([]solverEvidenceIdentity, 0, len(evidence))
	for id, digest := range evidence {
		values = append(values, solverEvidenceIdentity{ID: id, SHA256: digest})
	}
	sort.Slice(values, func(left, right int) bool { return values[left].ID < values[right].ID })
	return values
}

func (evidence solverEvidence) equal(other solverEvidence) bool {
	if len(evidence) != len(other) {
		return false
	}
	for id, digest := range evidence {
		if other[id] != digest {
			return false
		}
	}
	return true
}

func solverRequestEvidenceGrounded(
	schema cognition.ActionSchema,
	request cognition.ActionRequest,
	facts factSet,
	evidence solverEvidence,
	records []PublicRecord,
) bool {
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		return true
	}
	setID := EntityID(actionArgument(request, evidenceSetArg))
	if setID == "" {
		return false
	}
	recordHashes := make(map[EntityID]string, len(records))
	for _, record := range records {
		recordHashes[record.ID] = record.ContentSHA256
	}
	required := 0
	for _, fact := range facts {
		if fact.Name != "evidence.member" || len(fact.Args) != 2 || EntityID(fact.Args[0]) != setID {
			continue
		}
		id := EntityID(fact.Args[1])
		digest, exists := recordHashes[id]
		if !exists || evidence[id] != digest {
			return false
		}
		required++
	}
	return required > 0
}

func solverEvidenceAfterRequest(
	scenario Scenario,
	facts factSet,
	request cognition.ActionRequest,
	current solverEvidence,
) solverEvidence {
	next := current.clone()
	records, _ := observedRecords(
		scenario.descriptor.Records, scenario.artifactCorpus, facts, &request,
	)
	for _, record := range records {
		if record.Content != "" && textSHA256(record.Content) == record.ContentSHA256 {
			next[record.ID] = record.ContentSHA256
		}
	}
	return next
}

func solverStateKey(facts factSet, evidence solverEvidence) string {
	return canonicalJSON(struct {
		Facts    []cognition.Predicate    `json:"facts"`
		Evidence []solverEvidenceIdentity `json:"evidence"`
	}{Facts: facts.sorted(), Evidence: evidence.sorted()})
}
