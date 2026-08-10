package labyrinth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

const observationKind cognition.ObservationKind = "symbolic_state"

const MaxObservedRecords = 16

type publicObservationPayload struct {
	Format           string                `json:"format"`
	Predicates       []cognition.Predicate `json:"predicates"`
	Records          []ObservedRecord      `json:"records,omitempty"`
	RecordsTruncated bool                  `json:"records_truncated,omitempty"`
	GoalSatisfied    bool                  `json:"goal_satisfied"`
}

type ObservedRecord struct {
	ID            EntityID `json:"id"`
	Location      EntityID `json:"location"`
	Content       string   `json:"content,omitempty"`
	ContentSHA256 string   `json:"content_sha256"`
}

func buildObservation(
	actionID cognition.ActionID,
	revision cognition.WorldRevision,
	facts factSet,
	terminal bool,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
	records []PublicRecord,
	corpus *artifactCorpus,
	request *cognition.ActionRequest,
) (cognition.Observation, error) {
	visibleRecords, truncated := observedRecords(records, corpus, facts, request)
	content, err := publicObservationContent(
		facts, terminal, entities, schemas, visibleRecords, truncated,
	)
	if err != nil {
		return cognition.Observation{}, err
	}
	return newObservationFromContent(actionID, revision, content)
}

func newObservationFromContent(
	actionID cognition.ActionID,
	revision cognition.WorldRevision,
	content string,
) (cognition.Observation, error) {
	identity := sha256.Sum256([]byte(revision.SHA256 + "\x00" + string(actionID) + "\x00" + content))
	id := cognition.ObservationID("observation-" + hex.EncodeToString(identity[:]))
	if actionID == "" {
		return cognition.NewObservation(id, revision, observationKind, content)
	}
	return cognition.NewActionObservation(id, actionID, revision, observationKind, content)
}

func publicObservationContent(
	facts factSet,
	terminal bool,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
	records []ObservedRecord,
	recordsTruncated bool,
) (string, error) {
	public := make([]cognition.Predicate, 0, len(facts))
	current, hasCurrent := observationCurrentLocation(facts)
	for _, predicate := range facts {
		if predicateIsPublic(predicate, entities, schemas) &&
			topologyVisibleAt(predicate, current, hasCurrent) {
			public = append(public, predicate.Clone())
		}
	}
	sortPredicates(public)
	if len(public) > MaxPublicPredicates {
		return "", fmt.Errorf("%w: predicate count exceeds %d", ErrObservationLimit, MaxPublicPredicates)
	}
	payload := publicObservationPayload{
		Format: "symbolic-observation.v1", Predicates: public, Records: records,
		RecordsTruncated: recordsTruncated, GoalSatisfied: terminal,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode public observation: %w", err)
	}
	if len(raw) > cognition.MaxObservationBytes {
		return "", fmt.Errorf("%w: encoded observation exceeds %d bytes", ErrObservationLimit, cognition.MaxObservationBytes)
	}
	return string(raw), nil
}

func topologyVisibleAt(predicate cognition.Predicate, current string, hasCurrent bool) bool {
	if predicate.Name != "topology.edge" {
		return true
	}
	return hasCurrent && len(predicate.Args) == 2 && predicate.Args[0] == current
}

func observationCurrentLocation(facts factSet) (string, bool) {
	current := ""
	for _, fact := range facts {
		if fact.Name == "surface.marker" && len(fact.Args) == 1 {
			if current != "" && current != fact.Args[0] {
				return "", false
			}
			current = fact.Args[0]
		}
	}
	return current, current != ""
}

func observedRecords(
	records []PublicRecord,
	corpus *artifactCorpus,
	facts factSet,
	request *cognition.ActionRequest,
) ([]ObservedRecord, bool) {
	if request != nil && request.Kind != "observe" && request.Kind != "search" && request.Kind != "read" {
		return nil, false
	}
	if request != nil && request.Kind == "search" {
		return searchedRecords(records, corpus, actionArgument(*request, queryArg))
	}
	if request != nil && request.Kind == "read" {
		return readObservedRecord(records, corpus, EntityID(actionArgument(*request, artifactArg)))
	}
	location := EntityID("")
	if request == nil {
		if current, exists := observationCurrentLocation(facts); exists {
			location = EntityID(current)
		}
	} else if current, exists := observationCurrentLocation(facts); exists {
		location = EntityID(current)
	}
	result := make([]ObservedRecord, 0, MaxObservedRecords)
	for _, record := range records {
		if record.Location == location {
			result = append(result, observedRecord(record, false))
		}
	}
	visibleBase := 0
	for _, record := range records {
		if record.Location == location {
			visibleBase++
		}
	}
	additional, _ := corpus.recordsAt(location, false, MaxObservedRecords)
	result = append(result, additional...)
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	total := visibleBase + corpus.countAt(location)
	if len(result) > MaxObservedRecords {
		result = result[:MaxObservedRecords]
	}
	return result, total > len(result)
}

func searchedRecords(
	records []PublicRecord,
	corpus *artifactCorpus,
	query string,
) ([]ObservedRecord, bool) {
	if query == "" {
		return []ObservedRecord{}, false
	}
	result := make([]ObservedRecord, 0, MaxObservedRecords)
	total := 0
	for _, record := range records {
		if string(record.ID) != query && !strings.Contains(record.Content, query) {
			continue
		}
		total++
		result = append(result, observedRecord(record, true))
	}
	virtual, virtualTotal := corpus.search(query, MaxObservedRecords)
	total += virtualTotal
	result = append(result, virtual...)
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	if len(result) > MaxObservedRecords {
		result = result[:MaxObservedRecords]
	}
	return result, total > len(result)
}

func readObservedRecord(
	records []PublicRecord,
	corpus *artifactCorpus,
	id EntityID,
) ([]ObservedRecord, bool) {
	for _, record := range records {
		if record.ID == id {
			return []ObservedRecord{observedRecord(record, true)}, false
		}
	}
	if record, exists := corpus.recordByID(id); exists {
		return []ObservedRecord{observedRecord(record, true)}, false
	}
	return []ObservedRecord{}, false
}

func observedRecord(record PublicRecord, includeContent bool) ObservedRecord {
	content := ""
	if includeContent {
		content = record.Content
	}
	return ObservedRecord{
		ID: record.ID, Location: record.Location, Content: content,
		ContentSHA256: record.ContentSHA256,
	}
}

func buildPublicEffect(
	actionID cognition.ActionID,
	revision cognition.WorldRevision,
	changed bool,
) (cognition.Effect, error) {
	kind := cognition.EffectNoChange
	content := "The registered action committed without changing symbolic predicates."
	if changed {
		kind = cognition.EffectStateChanged
		content = "The registered action committed symbolic predicate changes."
	}
	return cognition.NewEffect(actionID, revision, kind, content)
}
