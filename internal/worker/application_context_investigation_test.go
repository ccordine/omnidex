package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationContextQuestionInventoryAbsenceReturnsCurrentContext(t *testing.T) {
	t.Parallel()
	const request = "Use the existing patient-search behavior."
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	resolverCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			input := decodeApplicationContextQuestionInventoryInput(t, job, modelName)
			if len(input.Context.Facts) != 1 {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected inventory input: %+v", input)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: assemblyline.ApplicationNoRepositoryFactQuestionCandidates,
			}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			resolverCalls++
			return nil, fmt.Errorf("empty inventory invoked the resolver")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || resolverCalls != 0 || !reflect.DeepEqual(resolved, initial) {
		t.Fatalf("model calls=%d resolver calls=%d context=%+v", modelCalls, resolverCalls, resolved)
	}
}

func TestApplicationContextQuestionQueueResolvesTwoNeedsWithEvolvingEvidence(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing patient search with its current access policy."
	questions := []string{
		"Which declaration owns patient filtering?",
		"Which existing policy constrains that declaration?",
	}
	facts := []string{
		"PatientQuery owns patient filtering.",
		"PatientVisibilityPolicy constrains PatientQuery results.",
	}
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	resolverCalls := 0
	necessityCalls := 0
	relationCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			switch job.Kind {
			case assemblyline.WorkApplicationContextQuestionInventory:
				input := decodeApplicationContextQuestionInventoryInput(t, job, modelName)
				if len(input.Context.Facts) != 1 {
					return assemblyline.PortableResult{}, fmt.Errorf("inventory context=%+v", input.Context)
				}
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: strings.Join(questions, "\n"),
				}, nil
			case assemblyline.WorkApplicationContextQuestionNecessity:
				input := decodeApplicationContextQuestionNecessityInput(t, job, modelName)
				prompt, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.CandidateIndex != necessityCalls ||
					len(input.CurrentContext.Facts) != necessityCalls+1 {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"necessity call %d authority=%+v", necessityCalls, input,
					)
				}
				for index := 0; index < necessityCalls; index++ {
					if input.CurrentContext.Facts[index+1].Value != facts[index] ||
						!strings.Contains(prompt, facts[index]) ||
						strings.Contains(prompt, questions[index]) {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"necessity call %d crossed accepted-question boundary %d", necessityCalls, index,
						)
					}
				}
				necessityCalls++
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionNecessary,
				}, nil
			case assemblyline.WorkApplicationContextQuestionRelation:
				input := decodeApplicationContextQuestionRelationInput(t, job, modelName)
				relationCalls++
				if necessityCalls != 2 {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"question relation ran before both candidates passed necessity: calls=%d",
						necessityCalls,
					)
				}
				if input.CandidateQuestion != questions[1] || input.AcceptedQuestion != questions[0] {
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected question pair: %+v", input)
				}
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionsDistinctFact,
				}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context job %q", job.Kind)
			}
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(need assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			index := resolverCalls
			resolverCalls++
			if index >= len(questions) || need.Question != questions[index] ||
				need.ID != fmt.Sprintf("context_evidence_need_%03d", index+1) {
				return nil, fmt.Errorf("unexpected evidence need %d: %+v", index, need)
			}
			return applicationContextEvidence(
				facts[index], fmt.Sprintf("symbol:context-%d", index+1),
			), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 4 || necessityCalls != 2 || relationCalls != 1 ||
		resolverCalls != 2 || len(resolved.Facts) != 3 {
		t.Fatalf(
			"model calls=%d necessity=%d relation=%d resolver=%d context=%+v",
			modelCalls, necessityCalls, relationCalls, resolverCalls, resolved,
		)
	}
}

func TestApplicationContextQuestionQueueSkipsExactAndSemanticRepeatsWithoutReview(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing patient search."
	const question = "Which declaration owns patient filtering?"
	const semanticRepeat = "What existing symbol is responsible for patient filtering?"
	const fact = "PatientQuery owns patient filtering."
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	resolverCalls := 0
	necessityByQuestion := make(map[string]int)
	relationByCandidate := make(map[string]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			switch job.Kind {
			case assemblyline.WorkApplicationContextQuestionInventory:
				decodeApplicationContextQuestionInventoryInput(t, job, modelName)
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: strings.Join([]string{question, question, semanticRepeat}, "\n"),
				}, nil
			case assemblyline.WorkApplicationContextQuestionNecessity:
				input := decodeApplicationContextQuestionNecessityInput(t, job, modelName)
				candidate := input.Inventory.Candidates[input.CandidateIndex]
				necessityByQuestion[candidate]++
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionNecessary,
				}, nil
			case assemblyline.WorkApplicationContextQuestionRelation:
				input := decodeApplicationContextQuestionRelationInput(t, job, modelName)
				relationByCandidate[input.CandidateQuestion]++
				if input.CandidateQuestion != semanticRepeat || input.AcceptedQuestion != question {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"semantic repeat lacked exact pair authority: %+v", input,
					)
				}
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionsSameFact,
				}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context job %q", job.Kind)
			}
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(need assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			resolverCalls++
			if need.Question != question {
				return nil, fmt.Errorf("unexpected resolved question: %+v", need)
			}
			return applicationContextEvidence(fact, "symbol:PatientQuery"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 4 || resolverCalls != 1 || len(resolved.Facts) != 2 {
		t.Fatalf("model calls=%d resolver calls=%d context=%+v", modelCalls, resolverCalls, resolved)
	}
	if necessityByQuestion[question] != 1 || necessityByQuestion[semanticRepeat] != 1 ||
		relationByCandidate[semanticRepeat] != 1 {
		t.Fatalf("necessity calls=%v", necessityByQuestion)
	}
}

func TestApplicationContextQuestionPairQueuePreservesAcceptedStateAcrossLaterDuplicate(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing invoice export with its current approval policy."
	questions := []string{
		"Which declaration owns invoice export?",
		"Which existing policy authorizes invoice export?",
		"What current authorization rule governs invoice export?",
	}
	facts := []string{
		"InvoiceExporter owns invoice export.",
		"InvoiceApprovalPolicy authorizes invoice export.",
	}
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	necessityCandidates := []string{}
	pairs := [][2]string{}
	resolverCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			switch job.Kind {
			case assemblyline.WorkApplicationContextQuestionInventory:
				decodeApplicationContextQuestionInventoryInput(t, job, modelName)
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: strings.Join(questions, "\n"),
				}, nil
			case assemblyline.WorkApplicationContextQuestionNecessity:
				input := decodeApplicationContextQuestionNecessityInput(t, job, modelName)
				candidate := input.Inventory.Candidates[input.CandidateIndex]
				necessityCandidates = append(necessityCandidates, candidate)
				if len(input.CurrentContext.Facts) != len(necessityCandidates) {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"necessity candidate %q context=%+v", candidate, input.CurrentContext,
					)
				}
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionNecessary,
				}, nil
			case assemblyline.WorkApplicationContextQuestionRelation:
				input := decodeApplicationContextQuestionRelationInput(t, job, modelName)
				pairs = append(pairs, [2]string{input.CandidateQuestion, input.AcceptedQuestion})
				relation := assemblyline.ApplicationContextQuestionsDistinctFact
				if input.CandidateQuestion == questions[2] && input.AcceptedQuestion == questions[1] {
					relation = assemblyline.ApplicationContextQuestionsSameFact
				}
				return assemblyline.PortableResult{JobID: job.ID, Candidate: relation}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context job %q", job.Kind)
			}
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(need assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			index := resolverCalls
			resolverCalls++
			if index >= 2 || need.Question != questions[index] {
				return nil, fmt.Errorf("unexpected resolved need: %+v", need)
			}
			return applicationContextEvidence(
				facts[index], fmt.Sprintf("symbol:invoice-%d", index+1),
			), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPairs := [][2]string{
		{questions[1], questions[0]},
		{questions[2], questions[0]},
		{questions[2], questions[1]},
	}
	if modelCalls != 7 || resolverCalls != 2 ||
		!reflect.DeepEqual(necessityCandidates, questions) ||
		!reflect.DeepEqual(pairs, wantPairs) || len(resolved.Facts) != 3 ||
		resolved.Facts[1].Value != facts[0] || resolved.Facts[2].Value != facts[1] {
		t.Fatalf(
			"model=%d resolver=%d necessities=%v pairs=%v context=%+v",
			modelCalls, resolverCalls, necessityCandidates, pairs, resolved,
		)
	}
}

func TestApplicationContextQuestionQueueDiscardsInventedNeedsOnExhaustion(t *testing.T) {
	t.Parallel()
	const request = "Preserve the existing patient search behavior."
	questions := []string{
		"Which optional dashboard color should be added?",
		"Which unrequested analytics system should be introduced?",
	}
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	resolverCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			switch job.Kind {
			case assemblyline.WorkApplicationContextQuestionInventory:
				decodeApplicationContextQuestionInventoryInput(t, job, modelName)
				return assemblyline.PortableResult{JobID: job.ID, Candidate: strings.Join(questions, "\n")}, nil
			case assemblyline.WorkApplicationContextQuestionNecessity:
				decodeApplicationContextQuestionNecessityInput(t, job, modelName)
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionNotNecessary,
				}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context job %q", job.Kind)
			}
		},
	}
	resolved, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			resolverCalls++
			return nil, fmt.Errorf("rejected question invoked resolver")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelCalls != 3 || resolverCalls != 0 || !reflect.DeepEqual(resolved, initial) {
		t.Fatalf("model calls=%d resolver calls=%d context=%+v", modelCalls, resolverCalls, resolved)
	}
}

func TestApplicationContextAuthorizedQuestionResolverFailureIsLoud(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing patient search."
	const question = "Which declaration owns patient filtering?"
	want := errors.New("repository index unavailable")
	initial := applicationContextInvestigationFixture(t, request)
	modelCalls := 0
	resolverCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			modelCalls++
			switch job.Kind {
			case assemblyline.WorkApplicationContextQuestionInventory:
				decodeApplicationContextQuestionInventoryInput(t, job, modelName)
				return assemblyline.PortableResult{JobID: job.ID, Candidate: question}, nil
			case assemblyline.WorkApplicationContextQuestionNecessity:
				decodeApplicationContextQuestionNecessityInput(t, job, modelName)
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: assemblyline.ApplicationContextQuestionNecessary,
				}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected context job %q", job.Kind)
			}
		},
	}
	_, err := resolveDirectCodingApplicationContext(
		runtime, "context-model", request, initial, nil,
		func(assemblyline.ApplicationEvidenceNeed) ([]assemblyline.ApplicationContextEvidence, error) {
			resolverCalls++
			return nil, want
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("resolver error=%v", err)
	}
	if modelCalls != 2 || resolverCalls != 1 {
		t.Fatalf("model calls=%d resolver calls=%d", modelCalls, resolverCalls)
	}
}

func applicationContextInvestigationFixture(
	t testing.TB,
	request string,
) assemblyline.ApplicationContext {
	t.Helper()
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func decodeApplicationContextQuestionInventoryInput(
	t testing.TB,
	job assemblyline.PortableJob,
	modelName string,
) assemblyline.ApplicationContextQuestionInventoryInput {
	t.Helper()
	if modelName != "context-model" || job.Kind != assemblyline.WorkApplicationContextQuestionInventory {
		t.Fatalf("unexpected context inventory call kind=%q model=%q", job.Kind, modelName)
	}
	var input assemblyline.ApplicationContextQuestionInventoryInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		t.Fatalf("decode application context question inventory payload: %v", err)
	}
	return input
}

func decodeApplicationContextQuestionNecessityInput(
	t testing.TB,
	job assemblyline.PortableJob,
	modelName string,
) assemblyline.ApplicationContextQuestionNecessityInput {
	t.Helper()
	if modelName != "context-model" || job.Kind != assemblyline.WorkApplicationContextQuestionNecessity {
		t.Fatalf("unexpected context necessity call kind=%q model=%q", job.Kind, modelName)
	}
	var input assemblyline.ApplicationContextQuestionNecessityInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		t.Fatalf("decode application context question necessity payload: %v", err)
	}
	return input
}

func decodeApplicationContextQuestionRelationInput(
	t testing.TB,
	job assemblyline.PortableJob,
	modelName string,
) assemblyline.ApplicationContextQuestionRelationInput {
	t.Helper()
	if modelName != "context-model" || job.Kind != assemblyline.WorkApplicationContextQuestionRelation {
		t.Fatalf("unexpected context question relation call kind=%q model=%q", job.Kind, modelName)
	}
	var input assemblyline.ApplicationContextQuestionRelationInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		t.Fatalf("decode application context question relation payload: %v", err)
	}
	return input
}

func applicationContextEvidence(
	value string,
	sourceID string,
) []assemblyline.ApplicationContextEvidence {
	return []assemblyline.ApplicationContextEvidence{{
		Value: value, SourceID: sourceID,
		SourceSHA256: assemblyline.ExactObjectiveContextSHA(value),
	}}
}
