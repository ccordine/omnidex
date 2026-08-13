package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
)

type desiredRepositoryGenerationResult struct {
	Candidates map[string]string
}

func (session *directCodingSession) generateDesiredRepositoryDeclarations(
	graph repositoryfacts.DesiredArtifactGraph,
) (desiredRepositoryGenerationResult, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return desiredRepositoryGenerationResult{}, fmt.Errorf("desired repository generation requires one active indexed session")
	}
	artifacts := append([]repositoryfacts.DesiredGoArtifact(nil), graph.Artifacts...)
	sort.Slice(artifacts, func(left, right int) bool { return artifacts[left].ID < artifacts[right].ID })
	requiresGeneration := false
	for _, artifact := range artifacts {
		if artifact.MustExist {
			requiresGeneration = true
			break
		}
	}
	if !requiresGeneration {
		return desiredRepositoryGenerationResult{Candidates: map[string]string{}}, nil
	}
	modelName, err := session.workerModel(station.CodingFragment)
	if err != nil {
		return desiredRepositoryGenerationResult{}, err
	}
	correctionModel, err := session.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return desiredRepositoryGenerationResult{}, err
	}
	runtime := directCodingWorkerRuntime(session)
	runtime.CorrectionModel = correctionModel
	originalExecute := runtime.Execute
	paths, err := desiredRepositoryTargetPaths(
		graph, session.repositoryIndex.Snapshot, session.repositoryIndex.Analyses,
	)
	if err != nil {
		return desiredRepositoryGenerationResult{}, err
	}
	runtime.Execute = func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		prompt, _, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if err := validateDesiredRepositoryModelEnvelope(prompt, paths); err != nil {
			return assemblyline.PortableResult{}, err
		}
		return originalExecute(job, model)
	}
	candidates := make(map[string]string)
	for _, artifact := range artifacts {
		if !artifact.MustExist {
			continue
		}
		if err := gofragment.RequireSelfContainedNewFunctionSignature(artifact.Signature); err != nil {
			return desiredRepositoryGenerationResult{}, fmt.Errorf(
				"desired artifact %q preflight: %w", artifact.ID, err,
			)
		}
		if len(artifact.ExistingSymbolIDs) != 0 {
			return desiredRepositoryGenerationResult{}, fmt.Errorf("desired existing artifact %q requires ordinary modification", artifact.ID)
		}
		if err := session.requireCurrentRepositoryAuthority("desired declaration generation"); err != nil {
			return desiredRepositoryGenerationResult{}, err
		}
		candidate, err := runDirectCodingGoFragmentGenerationWorker(
			runtime, modelName, directCodingGoGenerationJob{
				Subject: artifact.ID,
				Input: assemblyline.FragmentGenerationInput{
					Language: "go", Signature: artifact.Signature,
					Behavior:     artifact.RequirementQuote,
					Capabilities: []string{}, PermittedSymbols: []string{},
				},
			},
		)
		if err != nil {
			return desiredRepositoryGenerationResult{}, err
		}
		candidates[artifact.ID] = candidate
	}
	return desiredRepositoryGenerationResult{Candidates: candidates}, nil
}

func desiredRepositoryTargetPaths(
	graph repositoryfacts.DesiredArtifactGraph,
	snapshot repositoryfacts.Snapshot,
	analyses []repositoryfacts.Analysis,
) ([]string, error) {
	paths := make([]string, 0, len(graph.Artifacts))
	files := make(map[string]string, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file.Path
	}
	for _, artifact := range graph.Artifacts {
		if artifact.MustExist {
			compiled, err := gofragment.CompileNewFunctionSignature(artifact.Signature)
			if err != nil {
				return nil, err
			}
			name, err := deterministicGoSourceName(compiled.Name)
			if err != nil {
				return nil, err
			}
			analysis, err := exactRepositoryChangeAnalysis(
				analyses, desiredGraphAnalysisID(graph, analyses),
			)
			if err != nil {
				return nil, err
			}
			placement, err := repositoryfacts.ResolveGoPackagePlacement(
				snapshot, analysis, artifact.PackageArtifactID,
			)
			if err != nil {
				return nil, err
			}
			if placement.Directory != "." {
				name = path.Join(placement.Directory, name)
			}
			paths = append(paths, name)
			continue
		}
		for _, symbolID := range artifact.ExistingSymbolIDs {
			for _, analysis := range analyses {
				for _, symbol := range analysis.Symbols {
					if symbol.ID == symbolID {
						if value := files[symbol.FileID]; value != "" {
							paths = append(paths, value)
						}
					}
				}
			}
		}
	}
	return paths, nil
}

func validateDesiredRepositoryModelEnvelope(prompt string, paths []string) error {
	lower := strings.ToLower(prompt)
	for _, token := range []string{
		"create_file", "delete_file", "rename_file", "move_file", "write_file",
		" rm ", " mv ", "filesystem operation", "repository operation",
	} {
		if strings.Contains(lower, token) {
			return fmt.Errorf("desired repository model envelope exposes forbidden mutation authority %q", token)
		}
	}
	for _, target := range paths {
		if target != "" && strings.Contains(prompt, target) {
			return fmt.Errorf("desired repository model envelope exposes code-owned target path")
		}
	}
	return nil
}
