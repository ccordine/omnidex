package assemblyline

import (
	"fmt"
	"sort"
)

type TargetTreeTransitionKind string

const (
	TargetTreeCreate TargetTreeTransitionKind = "create"
	TargetTreeRetain TargetTreeTransitionKind = "retain"
	TargetTreeModify TargetTreeTransitionKind = "modify"
	TargetTreeMove   TargetTreeTransitionKind = "move"
	TargetTreeDelete TargetTreeTransitionKind = "delete"
)

type TargetTreeTransition struct {
	Kind       TargetTreeTransitionKind
	ArtifactID string
	FromPath   string
	ToPath     string
	Target     *ResolvedTargetArtifact
}

// DiffTargetTree derives all structure work. It does not inspect source or
// decide declaration content; those are subsequent code-owned stages.
func DiffTargetTree(input TargetTreeInput, target TargetTree) ([]TargetTreeTransition, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if len(target.Artifacts) == 0 {
		return nil, fmt.Errorf("target tree must contain at least one artifact")
	}
	current := make(map[string]CurrentTargetArtifact, len(input.Current))
	for _, artifact := range input.Current {
		current[artifact.ID] = artifact
	}
	seen := make(map[string]struct{}, len(target.Artifacts))
	transitions := make([]TargetTreeTransition, 0, len(input.Current)+len(target.Artifacts))
	for index := range target.Artifacts {
		artifact := target.Artifacts[index]
		if artifact.Existing {
			currentArtifact, exists := current[artifact.ID]
			if !exists {
				return nil, fmt.Errorf("target tree references current artifact %q that is absent", artifact.ID)
			}
			if _, duplicate := seen[artifact.ID]; duplicate {
				return nil, fmt.Errorf("target tree duplicates current artifact %q", artifact.ID)
			}
			seen[artifact.ID] = struct{}{}
			if currentArtifact.Path != artifact.Path {
				transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeMove, ArtifactID: artifact.ID, FromPath: currentArtifact.Path, ToPath: artifact.Path, Target: &artifact})
			} else if currentArtifact.Purpose == artifact.Purpose && equalTargetTreeRequirementIDs(currentArtifact.RequirementIDs, artifact.RequirementIDs) {
				transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeRetain, ArtifactID: artifact.ID, FromPath: currentArtifact.Path, ToPath: artifact.Path, Target: &artifact})
			} else {
				transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeModify, ArtifactID: artifact.ID, FromPath: currentArtifact.Path, ToPath: artifact.Path, Target: &artifact})
			}
			continue
		}
		if _, duplicate := seen[artifact.ID]; duplicate {
			return nil, fmt.Errorf("target tree duplicates new artifact %q", artifact.ID)
		}
		seen[artifact.ID] = struct{}{}
		transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeCreate, ArtifactID: artifact.ID, ToPath: artifact.Path, Target: &artifact})
	}
	for _, artifact := range input.Current {
		if _, present := seen[artifact.ID]; !present {
			transitions = append(transitions, TargetTreeTransition{Kind: TargetTreeDelete, ArtifactID: artifact.ID, FromPath: artifact.Path})
		}
	}
	sort.Slice(transitions, func(left, right int) bool {
		leftPath, rightPath := transitions[left].ToPath, transitions[right].ToPath
		if leftPath == "" {
			leftPath = transitions[left].FromPath
		}
		if rightPath == "" {
			rightPath = transitions[right].FromPath
		}
		if leftPath == rightPath {
			return transitions[left].Kind < transitions[right].Kind
		}
		return leftPath < rightPath
	})
	return transitions, nil
}

func equalTargetTreeRequirementIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
