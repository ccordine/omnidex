package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const ablationProjectionPolicyVersionV1 = "omnidex.offline-context-projection.v1"

type ablationMaterial struct {
	Ref        taskstate.Ref
	SourceRefs []taskstate.Ref
	Role       workingset.Role
	Authority  taskstate.Authority
	Content    string
	Priority   int
}

func buildAblationProjection(
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	variant Variant,
	call uint32,
	contextBytes int,
	materials []ablationMaterial,
	maxEvidenceItems int,
) (contextbuilder.Projection, error) {
	if len(materials) < 2 || len(materials) > 64 ||
		maxEvidenceItems < 0 || maxEvidenceItems > len(materials)-1 {
		return contextbuilder.Projection{}, fmt.Errorf("ablation projection requires bounded task and context materials")
	}
	owner := workingset.Owner{
		LedgerID: taskstate.LedgerID("ledger_" + ablationContentSHA256(string(episode.ID))),
		JobID:    actor.JobID, Generation: actor.Generation,
	}
	set, err := workingset.New(owner, workingset.Budget{
		MaxItems: len(materials), MaxBytes: contextBytes,
		MaxPinnedItems: 0, MaxPinnedBytes: 0,
	})
	if err != nil {
		return contextbuilder.Projection{}, err
	}
	resolved := make([]contextbuilder.Material, 0, len(materials))
	roles := make(map[workingset.Role]int)
	for index, material := range materials {
		if material.Content == "" || material.Ref.Hash != ablationContentSHA256(material.Content) {
			return contextbuilder.Projection{}, fmt.Errorf("ablation material %d content authority is invalid", index+1)
		}
		itemID := workingset.ItemID(fmt.Sprintf("projected-material-%03d", index+1))
		if _, err := set.Acquire(workingset.AcquireRequest{
			ID: itemID, Ref: material.Ref, Role: material.Role,
			Retention: workingset.RetentionJob, Scope: set.Scope(), Priority: material.Priority,
			ByteCost: len([]byte(material.Content)),
			Acquisition: workingset.Acquisition{
				Provider: workingset.ProviderEvidence, OperationID: fmt.Sprintf("station-call-%03d", call),
				Reason: "Select exact bounded state for the current decision.",
			},
		}); err != nil {
			return contextbuilder.Projection{}, fmt.Errorf("acquire ablation material %d: %w", index+1, err)
		}
		resolved = append(resolved, contextbuilder.Material{
			ItemID: itemID, CurrentRef: material.Ref,
			SourceRefs: append([]taskstate.Ref{}, material.SourceRefs...),
			Authority:  material.Authority, Content: material.Content,
			ByteCost: len([]byte(material.Content)),
		})
		roles[material.Role]++
	}
	if roles[workingset.RoleTask] != 1 || roles[workingset.RoleEvidence] < 1 {
		return contextbuilder.Projection{}, fmt.Errorf("ablation projection requires one task and at least one evidence material")
	}
	required := []contextbuilder.Selector{{
		ID: "current-task", Role: workingset.RoleTask, MinItems: 1, MaxItems: 1,
	}}
	optional := []contextbuilder.Selector{}
	evidence := contextbuilder.Selector{
		ID: "active-evidence", Role: workingset.RoleEvidence,
		MinItems: roles[workingset.RoleEvidence], MaxItems: roles[workingset.RoleEvidence],
	}
	if variant == VariantLedgerProjection {
		evidence.MinItems = 0
		evidence.MaxItems = maxEvidenceItems
		if maxEvidenceItems > 0 {
			optional = append(optional, evidence)
		}
	} else {
		required = append(required, evidence)
	}
	scope := materials[0].Ref
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: fmt.Sprintf("decision-station-call-%03d", call),
		Spec: contextbuilder.ContextSpec{
			Name: "offline-decision-context", Version: ablationProjectionPolicyVersionV1,
			ScopeRef: scope, Required: required, Optional: optional,
			AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityCode, taskstate.AuthorityToolEvidence},
			MaxItems:           len(materials), MaxBytes: contextBytes, MaxAcquisitionRounds: 0,
		},
		WorkingSet: set, Materials: resolved,
	})
	if err != nil {
		return contextbuilder.Projection{}, err
	}
	return projection, projection.Validate()
}

func ablationProjectionRef(projection contextbuilder.Projection) cognition.ContextProjectionRef {
	return cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.ID), SHA256: projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.WorkingSetID),
		WorkingSetVersion: projection.WorkingSetVersion, RendererVersion: projection.RendererVersion,
	}
}

func ablationObservationRef(observation cognition.Observation) taskstate.Ref {
	return taskstate.Ref{
		URI: "cognition:episode/" + string(observation.Revision.EpisodeID) +
			"/observation/" + string(observation.ID),
		Version: strconv.FormatUint(observation.Revision.Number, 10),
		Hash:    observation.ContentSHA256, Relation: taskstate.RefEvidence,
	}
}

func ablationContentRef(uri, version, content string, relation taskstate.RefRelation) taskstate.Ref {
	return taskstate.Ref{
		URI: uri, Version: version, Hash: ablationContentSHA256(content), Relation: relation,
	}
}

func ablationContentSHA256(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}
