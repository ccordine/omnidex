package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextbuilder"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	repositoryShadowSpecVersion = "shadow-v1"
	repositoryResearchVersion   = "omnidex.repository-research-need.v1"
)

type repositoryShadowSource struct {
	item     workingset.AcquireRequest
	material contextbuilder.Material
}

type repositoryShadowProjectionPlan struct {
	spec    contextbuilder.ContextSpec
	sources []repositoryShadowSource
}

func repositoryShadowEligible(kind assemblyline.WorkKind) bool {
	return kind == assemblyline.WorkRepositoryRetrieval ||
		kind == assemblyline.WorkRepositoryChangeSurface
}

func repositoryShadowWorkingSetBudget() workingset.Budget {
	return workingset.Budget{MaxItems: 2, MaxBytes: 32 * 1024}
}

func repositoryShadowPlan(job assemblyline.PortableJob) (repositoryShadowProjectionPlan, error) {
	sources, err := assemblyline.ResolveRepositoryProjectionSources(job)
	if err != nil {
		return repositoryShadowProjectionPlan{}, err
	}
	research, err := repositoryResearchShadowSource(sources.ResearchNeed)
	if err != nil {
		return repositoryShadowProjectionPlan{}, err
	}
	plan := repositoryShadowProjectionPlan{sources: []repositoryShadowSource{research}}
	plan.spec = contextbuilder.ContextSpec{
		Version: repositoryShadowSpecVersion,
		ScopeRef: taskstate.Ref{
			URI: "portable-work:job/" + job.ID, Version: job.Schema,
			Hash: job.ID, Relation: taskstate.RefConcerns,
		},
		Required: []contextbuilder.Selector{{
			ID: "research-need", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1,
		}},
		AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityUser},
		MaxItems:           1, MaxBytes: 64 * 1024, MaxAcquisitionRounds: 0,
	}
	switch job.Kind {
	case assemblyline.WorkRepositoryRetrieval:
		plan.spec.Name = "repository-retrieval-context"
	case assemblyline.WorkRepositoryChangeSurface:
		if sources.Evidence == nil {
			return repositoryShadowProjectionPlan{}, fmt.Errorf("repository change surface projection requires canonical evidence")
		}
		evidence, sourceErr := repositoryEvidenceShadowSource(*sources.Evidence)
		if sourceErr != nil {
			return repositoryShadowProjectionPlan{}, sourceErr
		}
		plan.sources = append(plan.sources, evidence)
		plan.spec.Name = "repository-change-surface-context"
		plan.spec.Required = append(plan.spec.Required, contextbuilder.Selector{
			ID: "repository-evidence", Role: workingset.RoleRepositoryEvidence, MinItems: 1, MaxItems: 1,
		})
		plan.spec.AllowedAuthorities = append(plan.spec.AllowedAuthorities, taskstate.AuthorityToolEvidence)
		plan.spec.MaxItems = 2
	default:
		return repositoryShadowProjectionPlan{}, fmt.Errorf("repository shadow projection rejects work kind %q", job.Kind)
	}
	return plan, nil
}

func repositoryResearchShadowSource(researchNeed string) (repositoryShadowSource, error) {
	if researchNeed == "" || !utf8.ValidString(researchNeed) || strings.ContainsRune(researchNeed, '\x00') {
		return repositoryShadowSource{}, fmt.Errorf("repository shadow research authority is empty or malformed")
	}
	digest := repositoryShadowDigest([]byte(researchNeed))
	ref := taskstate.Ref{
		URI: "portable-work:research-need/" + digest, Version: repositoryResearchVersion,
		Hash: digest, Relation: taskstate.RefSource,
	}
	return newRepositoryShadowSource(
		"research-need-"+digest, ref, workingset.RoleUserAuthority,
		workingset.ProviderUser, taskstate.AuthorityUser, 100, researchNeed,
	), nil
}

func repositoryEvidenceShadowSource(pack repositoryretrieval.EvidencePack) (repositoryShadowSource, error) {
	if err := pack.Validate(); err != nil {
		return repositoryShadowSource{}, fmt.Errorf("repository shadow evidence: %w", err)
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		return repositoryShadowSource{}, fmt.Errorf("encode repository shadow evidence: %w", err)
	}
	digest := repositoryShadowDigest(raw)
	ref := taskstate.Ref{
		URI: "repo:evidence-pack/" + pack.ID, Version: pack.Schema,
		Hash: digest, Relation: taskstate.RefEvidence,
	}
	return newRepositoryShadowSource(
		"repository-evidence-"+digest, ref, workingset.RoleRepositoryEvidence,
		workingset.ProviderRepository, taskstate.AuthorityToolEvidence, 90, string(raw),
	), nil
}

func newRepositoryShadowSource(
	id string,
	ref taskstate.Ref,
	role workingset.Role,
	provider workingset.Provider,
	authority taskstate.Authority,
	priority int,
	content string,
) repositoryShadowSource {
	request := workingset.AcquireRequest{
		ID: workingset.ItemID(id), Ref: ref, Role: role, Retention: workingset.RetentionJob,
		Priority: priority, ByteCost: len([]byte(content)),
		Acquisition: workingset.Acquisition{
			Provider: provider, OperationID: "context-projection-acquire-" + id,
			Reason: "Required by the current repository semantic context projection.",
		},
	}
	return repositoryShadowSource{item: request, material: contextbuilder.Material{
		ItemID: request.ID, CurrentRef: ref, Authority: authority,
		Content: content, ByteCost: request.ByteCost,
	}}
}

func repositoryShadowDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
