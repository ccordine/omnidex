package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

// extendDirectCodingPathProvenance merges code-owned artifact identities into
// the current path boundary. NewArtifactIdentityProvenance revalidates every
// exact path and deterministically rebuilds unambiguous basename ownership.
func extendDirectCodingPathProvenance(
	current assemblyline.ArtifactIdentityProvenance,
	paths ...string,
) (assemblyline.ArtifactIdentityProvenance, error) {
	merged := current.Paths()
	seen := make(map[string]struct{}, len(merged)+len(paths))
	for _, artifactPath := range merged {
		seen[artifactPath] = struct{}{}
	}
	for _, artifactPath := range paths {
		if _, duplicate := seen[artifactPath]; duplicate {
			continue
		}
		seen[artifactPath] = struct{}{}
		merged = append(merged, artifactPath)
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(merged)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"extend direct coding artifact provenance: %w", err,
		)
	}
	return provenance, nil
}

func (s *directCodingSession) extendPathProvenance(paths ...string) error {
	if s == nil {
		return fmt.Errorf("direct coding path provenance requires one session")
	}
	provenance, err := extendDirectCodingPathProvenance(s.pathProvenance, paths...)
	if err != nil {
		return err
	}
	s.pathProvenance = provenance
	return nil
}

func (s *directCodingSession) bindDirectCodingTargetTreePathProvenance(
	target assemblyline.TargetTree,
) error {
	return s.extendPathProvenance(target.Paths...)
}

func (s *directCodingSession) bindDirectCodingProgramPathProvenance(
	program directCodingProgram,
) error {
	paths := append([]string(nil), program.TargetTree.Paths...)
	for _, document := range program.Source.Documents {
		paths = append(paths, document.Path)
	}
	for _, file := range program.StaticFiles {
		paths = append(paths, file.Path)
	}
	return s.extendPathProvenance(paths...)
}
