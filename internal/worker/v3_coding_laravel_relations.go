package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func deriveGenericLaravelServiceRelations(
	program directCodingProgram,
	assembly directCodingAssembly,
) ([]directCodingArtifactPathRelation, error) {
	if program.StackID != laravelHTTPServiceAdapter {
		return nil, fmt.Errorf("Laravel relations require stack %s", laravelHTTPServiceAdapter)
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
		"Dockerfile", "bootstrap/app.php", "composer.json", "composer.lock",
		"docker-compose.yml", "nginx/nginx.conf", "public/index.php", "routes/web.php",
	}
	storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
	if err != nil {
		return nil, err
	}
	if storage.RequiresPostgreSQL() {
		required = append(required,
			"config/database.php", laravelStateMigrationPath, phpServiceStateVerificationPath,
		)
	}
	if phpServiceHasHTMLResponse(program.ServiceEndpoints) {
		required = append(required, "package.json", "package-lock.json", "resources/styles.css")
	}
	for _, artifactPath := range required {
		if _, exists := present[artifactPath]; !exists {
			return nil, fmt.Errorf("Laravel relation authority lacks artifact %s", artifactPath)
		}
	}
	relations := []directCodingArtifactPathRelation{
		{FromPath: "docker-compose.yml", ToPath: "Dockerfile", Kind: assemblyline.ArtifactRelationComposes},
		{FromPath: "docker-compose.yml", ToPath: "nginx/nginx.conf", Kind: assemblyline.ArtifactRelationComposes},
		{FromPath: "nginx/nginx.conf", ToPath: "public/index.php", Kind: assemblyline.ArtifactRelationRoutesTo},
		{FromPath: "public/index.php", ToPath: "bootstrap/app.php", Kind: assemblyline.ArtifactRelationConsumes},
		{FromPath: "bootstrap/app.php", ToPath: "routes/web.php", Kind: assemblyline.ArtifactRelationConsumes},
		{FromPath: "Dockerfile", ToPath: "composer.json", Kind: assemblyline.ArtifactRelationConsumes},
		{FromPath: "Dockerfile", ToPath: "composer.lock", Kind: assemblyline.ArtifactRelationConsumes},
	}
	if storage.RequiresPostgreSQL() {
		relations = append(relations,
			directCodingArtifactPathRelation{
				FromPath: "docker-compose.yml", ToPath: laravelStateMigrationPath,
				Kind: assemblyline.ArtifactRelationConsumes,
			},
			directCodingArtifactPathRelation{
				FromPath: laravelStateMigrationPath, ToPath: "config/database.php",
				Kind: assemblyline.ArtifactRelationConsumes,
			},
		)
	}
	for _, artifactPath := range phpPaths {
		relations = append(relations, directCodingArtifactPathRelation{
			FromPath: "Dockerfile", ToPath: artifactPath, Kind: assemblyline.ArtifactRelationConsumes,
		})
	}
	return relations, nil
}
