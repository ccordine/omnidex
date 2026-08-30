package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type repositoryRequirementCandidateSpan struct {
	start int
	end   int
}

func (span repositoryRequirementCandidateSpan) overlaps(
	other repositoryRequirementCandidateSpan,
) bool {
	return span.start < other.end && other.start < span.end
}

// repositoryRequirementCandidateQueue owns source order, exact duplicate
// suppression, overlap suppression, and exhaustion. Model output cannot add an
// entry, revisit a retained requirement, or decide whether the queue continues.
type repositoryRequirementCandidateQueue struct {
	authority     assemblyline.RepositoryRequirementInterpretationInput
	inventory     assemblyline.RepositoryRequirementInventory
	pending       []int
	seen          map[string]struct{}
	retained      []string
	retainedSpans []repositoryRequirementCandidateSpan
}

func newRepositoryRequirementCandidateQueue(
	authority assemblyline.RepositoryRequirementInterpretationInput,
	inventory assemblyline.RepositoryRequirementInventory,
) (*repositoryRequirementCandidateQueue, error) {
	if err := inventory.ValidateFor(authority); err != nil {
		return nil, err
	}
	owned := inventory
	owned.Candidates = append([]string(nil), inventory.Candidates...)
	pending := make([]int, len(owned.Candidates))
	for index := range pending {
		pending[index] = index
	}
	return &repositoryRequirementCandidateQueue{
		authority: authority,
		inventory: owned,
		pending:   pending,
		seen:      make(map[string]struct{}, len(pending)),
		retained:  make([]string, 0, assemblyline.MaxRepositoryRequirementLeaves),
	}, nil
}

func (queue *repositoryRequirementCandidateQueue) next() (
	assemblyline.RepositoryRequirementCandidateAuthorizationInput,
	bool,
	error,
) {
	for len(queue.pending) > 0 {
		index := queue.pending[0]
		queue.pending = queue.pending[1:]
		candidate := queue.inventory.Candidates[index]
		if _, duplicate := queue.seen[candidate]; duplicate {
			continue
		}
		queue.seen[candidate] = struct{}{}
		span, err := repositoryRequirementSourceSpan(
			queue.authority.UserRequest,
			candidate,
		)
		if err != nil {
			return assemblyline.RepositoryRequirementCandidateAuthorizationInput{}, false, err
		}
		overlapsRetained := false
		for _, retained := range queue.retainedSpans {
			if span.overlaps(retained) {
				overlapsRetained = true
				break
			}
		}
		if overlapsRetained {
			continue
		}
		input := assemblyline.RepositoryRequirementCandidateAuthorizationInput{
			Authority:      queue.authority,
			Inventory:      queue.inventory,
			CandidateIndex: index,
		}
		return input, true, input.Inventory.ValidateFor(input.Authority)
	}
	return assemblyline.RepositoryRequirementCandidateAuthorizationInput{}, false, nil
}

func (queue *repositoryRequirementCandidateQueue) retain(
	input assemblyline.RepositoryRequirementCandidateAuthorizationInput,
) error {
	if err := input.Inventory.ValidateFor(input.Authority); err != nil {
		return err
	}
	if input.Inventory.AuthoritySHA256 != queue.inventory.AuthoritySHA256 ||
		input.Inventory.RawSHA256 != queue.inventory.RawSHA256 ||
		input.CandidateIndex < 0 || input.CandidateIndex >= len(queue.inventory.Candidates) {
		return fmt.Errorf("repository requirement retention differs from queue authority")
	}
	candidate := queue.inventory.Candidates[input.CandidateIndex]
	if candidate != input.Inventory.Candidates[input.CandidateIndex] {
		return fmt.Errorf("repository requirement retention candidate differs from queue authority")
	}
	span, err := repositoryRequirementSourceSpan(queue.authority.UserRequest, candidate)
	if err != nil {
		return err
	}
	for _, retained := range queue.retainedSpans {
		if span.overlaps(retained) {
			return fmt.Errorf("repository requirement retention overlaps retained source authority")
		}
	}
	queue.retained = append(queue.retained, candidate)
	queue.retainedSpans = append(queue.retainedSpans, span)
	return nil
}

func (queue *repositoryRequirementCandidateQueue) requirements() []string {
	return append([]string(nil), queue.retained...)
}

func repositoryRequirementSourceSpan(
	source string,
	candidate string,
) (repositoryRequirementCandidateSpan, error) {
	start := strings.Index(source, candidate)
	if start < 0 {
		return repositoryRequirementCandidateSpan{}, fmt.Errorf(
			"repository requirement candidate %q is not an exact source substring",
			candidate,
		)
	}
	if strings.Contains(source[start+len(candidate):], candidate) {
		return repositoryRequirementCandidateSpan{}, fmt.Errorf(
			"repository requirement candidate %q is not uniquely grounded",
			candidate,
		)
	}
	return repositoryRequirementCandidateSpan{
		start: start,
		end:   start + len(candidate),
	}, nil
}

func interpretRepositoryRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	context assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	input := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: authority,
		Context:     context,
	}
	inventoryJob, err := assemblyline.NewRepositoryRequirementInventoryJob(input)
	if err != nil {
		return nil, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime,
		modelName,
		"repository_requirement_inventory",
		inventoryJob,
		identities,
		func(raw string) (assemblyline.RepositoryRequirementInventory, error) {
			return assemblyline.DecodeRepositoryRequirementInventory(input, raw)
		},
		func(value assemblyline.RepositoryRequirementInventory) error {
			if err := value.ValidateFor(input); err != nil {
				return err
			}
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"repository requirement inventory",
				runtime.PathProvenance,
				value.Candidates...,
			)
		},
	)
	if err != nil {
		return nil, err
	}
	queue, err := newRepositoryRequirementCandidateQueue(input, inventory)
	if err != nil {
		return nil, err
	}
	for {
		candidateInput, remains, err := queue.next()
		if err != nil {
			return nil, err
		}
		if !remains {
			break
		}
		job, err := assemblyline.NewRepositoryRequirementCandidateAuthorizationJob(candidateInput)
		if err != nil {
			return nil, err
		}
		relation, err := runDirectCodingSemanticLeafCall(
			runtime,
			modelName,
			"repository_requirement_candidate_authorization",
			job,
			identities,
			func(raw string) (assemblyline.RepositoryRequirementCandidateAuthorizationResult, error) {
				return assemblyline.DecodeRepositoryRequirementCandidateAuthorizationResult(
					candidateInput,
					raw,
				)
			},
			func(value assemblyline.RepositoryRequirementCandidateAuthorizationResult) error {
				return value.ValidateFor(candidateInput)
			},
		)
		if err != nil {
			return nil, err
		}
		if relation.Relation != assemblyline.RepositoryRequirementCandidateRequiresChange {
			continue
		}
		candidate := candidateInput.Inventory.Candidates[candidateInput.CandidateIndex]
		duplicate := false
		for _, retained := range queue.requirements() {
			relationInput := assemblyline.RepositoryRequirementCandidateRelationInput{
				Candidate: candidate, AcceptedRequirement: retained,
			}
			relationJob, err := assemblyline.NewRepositoryRequirementCandidateRelationJob(
				relationInput,
			)
			if err != nil {
				return nil, err
			}
			candidateRelation, err := runDirectCodingSemanticLeafCall(
				runtime,
				modelName,
				"repository_requirement_candidate_relation",
				relationJob,
				identities,
				func(raw string) (assemblyline.RepositoryRequirementCandidateRelationResult, error) {
					return assemblyline.DecodeRepositoryRequirementCandidateRelationResult(
						relationInput,
						raw,
					)
				},
				func(value assemblyline.RepositoryRequirementCandidateRelationResult) error {
					return value.ValidateFor(relationInput)
				},
			)
			if err != nil {
				return nil, err
			}
			if candidateRelation.Relation == assemblyline.RepositoryRequirementCandidatesSameChange {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if err := queue.retain(candidateInput); err != nil {
			return nil, err
		}
	}
	requirements := queue.requirements()
	if len(requirements) == 0 {
		return nil, fmt.Errorf(
			"repository requirement candidate queue exhausted without one requested existing-workspace change",
		)
	}
	interpretation := assemblyline.RepositoryRequirementInterpretation{
		Schema:       assemblyline.RepositoryRequirementInterpretationSchemaV3,
		Requirements: requirements,
	}
	return assemblyline.ResolveRepositoryRequirements(input, interpretation)
}
