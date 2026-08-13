package worker

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestObjectiveRepositoryEvidenceRequiresRelevanceForDeterministicHit(t *testing.T) {
	queries := []string{}
	termCalls, relevanceCalls := 0, 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"exact implementation owner",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termCalls++
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, errors.New("must not run")
		},
		func(requirement string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceCalls++
			if requirement != "exact implementation owner" || len(evidence) != 1 {
				t.Fatalf("requirement=%q evidence=%#v", requirement, evidence)
			}
			return selectedRepositoryEvidence(evidence[0].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{"exact implementation owner"}) ||
		termCalls != 0 || relevanceCalls != 1 ||
		result.ModelCalls != 1 || len(result.Evidence) != 1 {
		t.Fatalf("queries=%v term_calls=%d relevance_calls=%d result=%#v", queries, termCalls, relevanceCalls, result)
	}
}

func TestObjectiveRepositoryEvidenceOpensExactlyOneSearchTermGapAfterMiss(t *testing.T) {
	queries := []string{}
	termCalls, relevanceCalls := 0, 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"invitation timing owner",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			if query == "delivery scheduler" {
				return repositoryAcquisitionTestPack(t, query), nil
			}
			return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termCalls++
			if unresolved != "invitation timing owner" {
				t.Fatalf("unresolved=%q", unresolved)
			}
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "delivery scheduler",
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(_ string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceCalls++
			return selectedRepositoryEvidence(evidence[0].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{"invitation timing owner", "delivery scheduler"}) ||
		termCalls != 1 || relevanceCalls != 1 ||
		result.ModelCalls != 2 || len(result.Evidence) != 1 {
		t.Fatalf("queries=%v term_calls=%d relevance_calls=%d result=%#v", queries, termCalls, relevanceCalls, result)
	}
}

func TestObjectiveRepositoryEvidenceSecondMissFailsWithoutAnotherGap(t *testing.T) {
	buildCalls, termCalls, relevanceCalls := 0, 0, 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"unresolved owner",
		func(string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termCalls++
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "one alternate",
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(string, []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceCalls++
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, errors.New("must not run")
		},
	)
	if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) ||
		buildCalls != 2 || termCalls != 1 || relevanceCalls != 0 || result.ModelCalls != 0 || len(result.Evidence) != 0 {
		t.Fatalf("error=%v build_calls=%d term_calls=%d relevance_calls=%d result=%#v", err, buildCalls, termCalls, relevanceCalls, result)
	}
}

func TestObjectiveRepositoryEvidenceExpandsOnceAfterRelevantStationReturnsNone(t *testing.T) {
	queries := []string{}
	termCalls, relevanceCalls := 0, 0
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		"which implementation owns dispatch",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(exact string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termCalls++
			if exact != "which implementation owns dispatch" {
				t.Fatalf("exact=%q", exact)
			}
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "dispatch coordinator",
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(_ string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceCalls++
			if relevanceCalls == 1 {
				return assemblyline.RepositoryEvidenceRelevanceDecision{
					Schema:  assemblyline.RepositoryEvidenceRelevanceSchemaV1,
					Outcome: assemblyline.RepositoryEvidenceNone, EvidenceIDs: []string{},
				}, objectiveStationReceipt{Calls: 1}, nil
			}
			return selectedRepositoryEvidence(evidence[0].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{"which implementation owns dispatch", "dispatch coordinator"}) ||
		termCalls != 1 || relevanceCalls != 2 ||
		result.ModelCalls != 3 || len(result.Evidence) != 1 {
		t.Fatalf("queries=%v term_calls=%d relevance_calls=%d result=%#v", queries, termCalls, relevanceCalls, result)
	}
}

func TestObjectiveRepositoryEvidenceSecondRelevanceNoneFailsLoudly(t *testing.T) {
	buildCalls, termCalls, relevanceCalls := 0, 0, 0
	_, err := acquireObjectiveRepositoryEvidenceClosure(
		"which implementation owns dispatch",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termCalls++
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "alternate dispatch term",
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(string, []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceCalls++
			return assemblyline.RepositoryEvidenceRelevanceDecision{
				Schema:  assemblyline.RepositoryEvidenceRelevanceSchemaV1,
				Outcome: assemblyline.RepositoryEvidenceNone, EvidenceIDs: []string{},
			}, objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) || buildCalls != 2 || termCalls != 1 || relevanceCalls != 2 {
		t.Fatalf("error=%v build_calls=%d term_calls=%d relevance_calls=%d", err, buildCalls, termCalls, relevanceCalls)
	}
}

func TestObjectiveRepositoryEvidenceLongAuthorityUsesTermsFirstWithoutTruncation(t *testing.T) {
	exact := strings.TrimSpace(strings.Repeat("long objective ", 43))
	queries := []string{}
	termInput := ""
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		exact,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termInput = unresolved
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "bounded owner symbol",
			}, objectiveStationReceipt{Calls: 2}, nil
		},
		func(requirement string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			if requirement != exact {
				t.Fatalf("relevance requirement was rewritten: got %d bytes want %d", len(requirement), len(exact))
			}
			return selectedRepositoryEvidence(evidence[0].Capsule.ID), objectiveStationReceipt{Calls: 3}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if termInput != exact || !reflect.DeepEqual(queries, []string{"bounded owner symbol"}) ||
		result.ModelCalls != 5 {
		t.Fatalf("term preserved=%t queries=%v result=%#v", termInput == exact, queries, result)
	}
}

func TestObjectiveRepositoryEvidenceRejectsInventedCallLedger(t *testing.T) {
	t.Parallel()
	evidence := []objectiveEvidence{mustObjectiveEvidence(t, "R01", "bounded", "repository_symbol", "pack#symbol")}
	for _, invalid := range []objectiveEvidenceAcquisition{
		{Evidence: evidence, ModelCalls: 0},
		{
			Evidence: evidence, ModelCalls: maxObjectiveRepositoryEvidenceModelCalls + 1,
			RepositoryCallLedger: objectiveRepositoryAcquisitionCallLedger{
				searchTermCalls: maxTypedWorkerAttempts,
				relevanceCalls:  []int{maxTypedWorkerAttempts, maxTypedWorkerAttempts},
			},
		},
		{
			Evidence: evidence, ModelCalls: 2,
			RepositoryCallLedger: objectiveRepositoryAcquisitionCallLedger{relevanceCalls: []int{1}},
		},
	} {
		if err := validateObjectiveRepositoryEvidenceAcquisition(invalid); err == nil {
			t.Fatalf("invalid ledger accepted: %#v", invalid)
		}
	}
}

func TestObjectiveRepositoryEvidencePreservesExactWhitespaceForSemanticStations(t *testing.T) {
	exact := "  which implementation owns dispatch?  \n"
	termInput, relevanceInput := "", ""
	_, err := acquireObjectiveRepositoryEvidenceClosure(
		exact,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			if query != strings.TrimSpace(exact) && query != "dispatch owner" {
				t.Fatalf("deterministic query=%q", query)
			}
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			termInput = unresolved
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "dispatch owner",
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(requirement string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			relevanceInput = requirement
			if termInput == "" {
				return assemblyline.RepositoryEvidenceRelevanceDecision{
					Schema:  assemblyline.RepositoryEvidenceRelevanceSchemaV1,
					Outcome: assemblyline.RepositoryEvidenceNone, EvidenceIDs: []string{},
				}, objectiveStationReceipt{Calls: 1}, nil
			}
			return selectedRepositoryEvidence(evidence[0].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if termInput != exact || relevanceInput != exact {
		t.Fatalf("exact authority changed: term=%q relevance=%q", termInput, relevanceInput)
	}
}

func TestObjectiveRepositoryQueryDerivesBoundedSearchWithoutRewritingAuthority(t *testing.T) {
	t.Parallel()

	authority := turnAuthority{Instruction: "  Explain ReadThing.  \n"}
	query, err := objectiveRepositoryQuery(authority)
	if err != nil {
		t.Fatal(err)
	}
	if query != "Explain ReadThing." || authority.Instruction != "  Explain ReadThing.  \n" {
		t.Fatalf("query=%q authority=%q", query, authority.Instruction)
	}
	if _, err := objectiveRepositoryQuery(turnAuthority{Instruction: "  "}); err == nil {
		t.Fatal("blank deterministic repository query was accepted")
	}
	long := strings.Repeat("x", 513)
	query, err = objectiveRepositoryQuery(turnAuthority{Instruction: long})
	if err != nil || query != long {
		t.Fatalf("valid long authority rejected or changed: query_bytes=%d error=%v", len(query), err)
	}
	if _, err := objectiveRepositoryQuery(turnAuthority{Instruction: strings.Repeat("x", 4097)}); err == nil {
		t.Fatal("authority beyond the free-form boundary was accepted")
	}
}

func selectedRepositoryEvidence(id string) assemblyline.RepositoryEvidenceRelevanceDecision {
	return assemblyline.RepositoryEvidenceRelevanceDecision{
		Schema:  assemblyline.RepositoryEvidenceRelevanceSchemaV1,
		Outcome: assemblyline.RepositoryEvidenceRelevant, EvidenceIDs: []string{id},
	}
}
