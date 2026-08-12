package semanticreview

import (
	"bytes"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

const artifactSchemaV1 = "semantic-review-artifact.v1"

func NewInitialArtifact(
	objectiveID cognitionreference.ObjectiveID,
	content []byte,
) (Artifact, error) {
	return newArtifact(objectiveID, "", 1, content)
}

func newCorrectionArtifact(previous Artifact, value ArtifactValue) (Artifact, error) {
	if err := validateArtifact(previous); err != nil {
		return Artifact{}, err
	}
	if !validExactBytes(value.Content, maxArtifactBytes) {
		return Artifact{}, fmt.Errorf("%w: correction artifact content exceeds bounds", ErrInvalidArtifact)
	}
	if bytes.Equal(previous.Content, value.Content) {
		return Artifact{}, fmt.Errorf("%w: correction returned an unchanged artifact", ErrInvalidArtifact)
	}
	return newArtifact(previous.RootObjectiveID, previous.ID, previous.Revision+1, value.Content)
}

func newArtifact(
	objectiveID cognitionreference.ObjectiveID,
	parent ArtifactID,
	revision uint32,
	content []byte,
) (Artifact, error) {
	if !validIdentity(string(objectiveID)) {
		return Artifact{}, fmt.Errorf("%w: root objective ID is invalid", ErrInvalidArtifact)
	}
	if revision == 0 || (revision == 1) != (parent == "") ||
		(parent != "" && !validIdentity(string(parent))) {
		return Artifact{}, fmt.Errorf("%w: lineage is invalid", ErrInvalidArtifact)
	}
	if !validExactBytes(content, maxArtifactBytes) {
		return Artifact{}, fmt.Errorf("%w: content must be nonempty bounded exact UTF-8", ErrInvalidArtifact)
	}
	owned := bytes.Clone(content)
	if !validExactBytes(owned, maxArtifactBytes) {
		return Artifact{}, fmt.Errorf("%w: owned content is invalid", ErrInvalidArtifact)
	}
	artifact := Artifact{
		RootObjectiveID: objectiveID, ParentID: parent, Revision: revision,
		SHA256: digestBytes(owned), Content: owned,
	}
	artifact.ID = artifactIdentity(artifact)
	return cloneArtifact(artifact), nil
}

func artifactIdentity(value Artifact) ArtifactID {
	return artifactIdentityFromAuthority(
		value.RootObjectiveID, value.ParentID, value.Revision, value.SHA256,
	)
}

func artifactIdentityFromAuthority(
	root cognitionreference.ObjectiveID,
	parent ArtifactID,
	revision uint32,
	sha256 string,
) ArtifactID {
	return ArtifactID("A" + digestFields(
		artifactSchemaV1, string(root), string(parent), fmt.Sprintf("%d", revision), sha256,
	))
}

func validateArtifact(value Artifact) error {
	if !validIdentity(string(value.RootObjectiveID)) ||
		value.Revision == 0 || (value.Revision == 1) != (value.ParentID == "") ||
		(value.ParentID != "" && !validIdentity(string(value.ParentID))) ||
		!validExactBytes(value.Content, maxArtifactBytes) ||
		value.SHA256 != digestBytes(value.Content) || value.ID != artifactIdentity(value) {
		return fmt.Errorf("%w: artifact does not match exact content and lineage", ErrInvalidArtifact)
	}
	return nil
}

func cloneArtifact(value Artifact) Artifact {
	value.Content = bytes.Clone(value.Content)
	return value
}
