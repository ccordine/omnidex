package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestObjectiveRepositoryEvidenceDerivesNoneAndSelectionFromEvidenceIDs(t *testing.T) {
	t.Parallel()

	const requirement = "Which component owns dispatch?"
	queries := make([]string, 0, 2)
	searchTermCalls := 0
	relevanceCalls := 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		requirement,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			searchTermCalls++
			if unresolved != requirement {
				t.Fatalf("unresolved requirement=%q", unresolved)
			}
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV2, Anchors: []string{"Owner"},
			}, objectiveStationReceipt{Calls: 3}, nil
		},
		func(exactRequirement string, evidence []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			relevanceCalls++
			if exactRequirement != requirement || len(evidence) != 1 || evidence[0].Capsule.ID != "R01" {
				t.Fatalf("relevance projection requirement=%q evidence=%#v", exactRequirement, evidence)
			}
			ids := []string{}
			if relevanceCalls == 2 {
				ids = []string{"R01"}
			}
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema: assemblyline.RepositoryEvidenceRelevanceSchemaV1, EvidenceIDs: ids,
			}, objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 || queries[0] != requirement || queries[1] != `"owner"` ||
		searchTermCalls != 1 || relevanceCalls != 2 || result.ModelCalls != 5 ||
		len(result.Evidence) != 1 || result.Evidence[0].Capsule.ID != "R01" ||
		result.RepositoryCallLedger.searchTermCalls != 3 ||
		!reflect.DeepEqual(result.RepositoryCallLedger.relevanceCalls, []int{1, 1}) {
		t.Fatalf(
			"queries=%v search_term_calls=%d relevance_calls=%d result=%#v",
			queries, searchTermCalls, relevanceCalls, result,
		)
	}
}
