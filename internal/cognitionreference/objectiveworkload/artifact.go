package objectiveworkload

import "fmt"

const artifactSchemaV1 = "objective-workload-artifact.v1"

func newArtifact(workload Workload, item WorkItem, value ArtifactValue) (Artifact, error) {
	if value.Kind != ArtifactRequirementOutput {
		return Artifact{}, fmt.Errorf("%w: unsupported kind %q", ErrArtifact, value.Kind)
	}
	if len(value.Content) == 0 || len(value.Content) > maxArtifactBytes {
		return Artifact{}, fmt.Errorf(
			"%w: opaque content must contain between 1 and %d bytes",
			ErrArtifact, maxArtifactBytes,
		)
	}
	if item.WorkloadID != workload.ID || item.AuthoritySHA256 != workload.Authority.SHA256 {
		return Artifact{}, fmt.Errorf("%w: work item is not bound to workload authority", ErrArtifact)
	}
	content := append([]byte{}, value.Content...)
	contentDigest := digestBytes(content)
	artifact := Artifact{
		Kind: value.Kind, WorkloadID: workload.ID, RequirementID: item.Requirement.ID,
		AuthoritySHA256:   workload.Authority.SHA256,
		RequirementSHA256: item.Requirement.SHA256,
		RequirementStart:  item.Requirement.Start, RequirementEnd: item.Requirement.End,
		ContentSHA256: contentDigest, Content: content,
	}
	artifact.ID = ArtifactID("A" + digestFields(
		artifactSchemaV1, string(artifact.Kind), string(artifact.WorkloadID),
		string(artifact.RequirementID), artifact.AuthoritySHA256, artifact.RequirementSHA256,
		fmt.Sprintf("%d", artifact.RequirementStart), fmt.Sprintf("%d", artifact.RequirementEnd),
		artifact.ContentSHA256,
	))
	if err := validateArtifact(workload, item.Requirement, artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func validateArtifact(workload Workload, requirement Requirement, artifact Artifact) error {
	if artifact.Kind != ArtifactRequirementOutput || artifact.WorkloadID != workload.ID ||
		artifact.RequirementID != requirement.ID || artifact.AuthoritySHA256 != workload.Authority.SHA256 ||
		artifact.RequirementSHA256 != requirement.SHA256 || artifact.RequirementStart != requirement.Start ||
		artifact.RequirementEnd != requirement.End ||
		len(artifact.Content) == 0 || len(artifact.Content) > maxArtifactBytes ||
		artifact.ContentSHA256 != digestBytes(artifact.Content) {
		return fmt.Errorf("%w: artifact is not bound to exact workload state", ErrArtifact)
	}
	wantID := ArtifactID("A" + digestFields(
		artifactSchemaV1, string(artifact.Kind), string(artifact.WorkloadID),
		string(artifact.RequirementID), artifact.AuthoritySHA256, artifact.RequirementSHA256,
		fmt.Sprintf("%d", artifact.RequirementStart), fmt.Sprintf("%d", artifact.RequirementEnd),
		artifact.ContentSHA256,
	))
	if artifact.ID != wantID {
		return fmt.Errorf("%w: artifact identity does not match its content", ErrArtifact)
	}
	return nil
}
