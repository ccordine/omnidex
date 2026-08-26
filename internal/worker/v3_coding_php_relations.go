package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func deriveGenericPHPServiceRelations(
	program directCodingProgram,
	assembly directCodingAssembly,
) ([]directCodingArtifactPathRelation, error) {
	if program.StackID != genericPHPServiceAdapter {
		return nil, fmt.Errorf("PHP service relations require stack %s", genericPHPServiceAdapter)
	}
	present := make(map[string]struct{}, len(assembly.Files))
	phpPaths := make([]string, 0)
	for _, file := range assembly.Files {
		present[file.Path] = struct{}{}
		if strings.HasSuffix(strings.ToLower(file.Path), ".php") {
			phpPaths = append(phpPaths, file.Path)
		}
	}
	required := []string{
		"Dockerfile", "docker-compose.yml", "nginx/nginx.conf", "public/index.php", "composer.json",
	}
	hasHTML := phpServiceHasHTMLResponse(program.ServiceEndpoints)
	if hasHTML {
		required = append(required, "package.json", "package-lock.json", "resources/styles.css")
	}
	for _, artifactPath := range required {
		if _, exists := present[artifactPath]; !exists {
			return nil, fmt.Errorf("PHP service relation authority lacks artifact %s", artifactPath)
		}
	}
	if len(phpPaths) == 0 {
		return nil, fmt.Errorf("PHP service relation authority lacks PHP source artifacts")
	}
	relations := []directCodingArtifactPathRelation{
		{FromPath: "docker-compose.yml", ToPath: "Dockerfile", Kind: assemblyline.ArtifactRelationComposes},
		{FromPath: "docker-compose.yml", ToPath: "nginx/nginx.conf", Kind: assemblyline.ArtifactRelationComposes},
		{FromPath: "nginx/nginx.conf", ToPath: "public/index.php", Kind: assemblyline.ArtifactRelationRoutesTo},
		{FromPath: "Dockerfile", ToPath: "composer.json", Kind: assemblyline.ArtifactRelationConsumes},
	}
	if hasHTML {
		relations = append(relations,
			directCodingArtifactPathRelation{FromPath: "Dockerfile", ToPath: "package.json", Kind: assemblyline.ArtifactRelationConsumes},
			directCodingArtifactPathRelation{FromPath: "Dockerfile", ToPath: "package-lock.json", Kind: assemblyline.ArtifactRelationConsumes},
			directCodingArtifactPathRelation{FromPath: "Dockerfile", ToPath: "resources/styles.css", Kind: assemblyline.ArtifactRelationConsumes},
		)
	}
	for _, artifactPath := range phpPaths {
		relations = append(relations, directCodingArtifactPathRelation{
			FromPath: "Dockerfile", ToPath: artifactPath, Kind: assemblyline.ArtifactRelationConsumes,
		})
	}
	return relations, nil
}
