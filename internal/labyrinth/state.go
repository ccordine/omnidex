package labyrinth

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

type factSet map[string]cognition.Predicate

func newFactSet(values []cognition.Predicate) factSet {
	set := make(factSet, len(values))
	for _, predicate := range values {
		set[predicateKey(predicate)] = predicate.Clone()
	}
	return set
}

func (facts factSet) clone() factSet {
	cloned := make(factSet, len(facts))
	for key, predicate := range facts {
		cloned[key] = predicate.Clone()
	}
	return cloned
}

func (facts factSet) contains(predicate cognition.Predicate) bool {
	_, exists := facts[predicateKey(predicate)]
	return exists
}

func (facts factSet) sorted() []cognition.Predicate {
	values := make([]cognition.Predicate, 0, len(facts))
	for _, predicate := range facts {
		values = append(values, predicate.Clone())
	}
	sortPredicates(values)
	return values
}

func sortPredicates(values []cognition.Predicate) {
	type keyedPredicate struct {
		key       string
		predicate cognition.Predicate
	}
	keyed := make([]keyedPredicate, len(values))
	for index, predicate := range values {
		keyed[index] = keyedPredicate{key: predicateKey(predicate), predicate: predicate}
	}
	sort.Slice(keyed, func(left, right int) bool { return keyed[left].key < keyed[right].key })
	for index := range keyed {
		values[index] = keyed[index].predicate
	}
}

func goalSatisfied(goal cognition.GoalExpression, facts factSet) bool {
	for _, predicate := range goal.All {
		if !facts.contains(predicate) {
			return false
		}
	}
	if len(goal.Any) != 0 {
		matched := false
		for _, predicate := range goal.Any {
			if facts.contains(predicate) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, predicate := range goal.Not {
		if facts.contains(predicate) {
			return false
		}
	}
	return true
}

func revisionFor(
	scenario cognition.ScenarioRef,
	definitionSHA256 string,
	episode cognition.EpisodeRef,
	number uint64,
	previous string,
	actionID cognition.ActionID,
	requestSHA256 string,
	surfaceSHA256 string,
	facts factSet,
	totalCost int64,
	terminal bool,
) (cognition.WorldRevision, error) {
	payload := struct {
		Format           string                `json:"format"`
		Scenario         cognition.ScenarioRef `json:"scenario"`
		DefinitionSHA256 string                `json:"definition_sha256"`
		Episode          cognition.EpisodeRef  `json:"episode"`
		Number           uint64                `json:"number"`
		Previous         string                `json:"previous_sha256,omitempty"`
		ActionID         cognition.ActionID    `json:"action_id,omitempty"`
		RequestSHA256    string                `json:"request_sha256,omitempty"`
		SurfaceSHA256    string                `json:"surface_sha256,omitempty"`
		Facts            []cognition.Predicate `json:"facts"`
		TotalCost        int64                 `json:"total_cost"`
		Terminal         bool                  `json:"terminal"`
	}{
		"symbolic-revision.v1", scenario, definitionSHA256, episode, number, previous,
		actionID, requestSHA256, surfaceSHA256, facts.sorted(), totalCost, terminal,
	}
	digest, raw, err := digestJSON(payload)
	if err != nil {
		return cognition.WorldRevision{}, fmt.Errorf("encode symbolic revision: %w", err)
	}
	if len(raw) > MaxRevisionBytes {
		return cognition.WorldRevision{}, ErrWorldLimit
	}
	revision := cognition.WorldRevision{EpisodeID: episode.ID, Number: number, SHA256: digest}
	if err := revision.Validate(); err != nil {
		return cognition.WorldRevision{}, err
	}
	return revision, nil
}
