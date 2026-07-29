package worker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptFragmentJob struct {
	block          assemblyline.TypeScriptBlock
	tsx            bool
	available      string
	current        string
	failure        string
	requiredChange string
}

func generateDirectCodingTypeScriptFragments(
	runtime typedWorkerRuntime,
	modelName string,
	blueprint assemblyline.TypeScriptBlueprint,
) (map[string]string, error) {
	if err := validateDirectCodingFragmentConcurrency(runtime.MaxConcurrency); err != nil {
		return nil, err
	}
	waves, err := blueprint.BuildWaves()
	if err != nil {
		return nil, err
	}
	accepted := make(map[string]string)
	declarations := make(map[string]string)
	for _, wave := range waves {
		jobs := make([]directCodingTypeScriptFragmentJob, 0, len(wave))
		for _, ref := range wave {
			if !ref.Block.Generated() {
				declarations[ref.Block.ID] = strings.TrimSpace(ref.Block.API)
				continue
			}
			available, err := directCodingTypeScriptAvailableDeclarations(ref.Block, declarations)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, directCodingTypeScriptFragmentJob{
				block: ref.Block, tsx: ref.Document.TSX(), available: available,
			})
		}
		results := runDirectCodingTypeScriptFragmentWave(runtime, modelName, jobs)
		for _, result := range results {
			if result.err != nil {
				return nil, fmt.Errorf("generate block %s: %w", result.blockID, result.err)
			}
			accepted[result.blockID] = result.source
			block, exists := directCodingTypeScriptBlueprintBlock(blueprint, result.blockID)
			if !exists {
				return nil, fmt.Errorf("generated TypeScript block %s is absent from its blueprint", result.blockID)
			}
			declarations[result.blockID] = strings.TrimSpace(block.API)
		}
	}
	return accepted, nil
}

func runDirectCodingTypeScriptFragmentWave(
	runtime typedWorkerRuntime,
	modelName string,
	jobs []directCodingTypeScriptFragmentJob,
) []directCodingFragmentResult {
	results := make([]directCodingFragmentResult, len(jobs))
	if runtime.MaxConcurrency == 1 {
		for index, job := range jobs {
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, modelName, job)
			results[index] = directCodingFragmentResult{blockID: job.block.ID, source: source, err: err}
			if err != nil {
				break
			}
		}
		return results
	}
	semaphore := make(chan struct{}, runtime.MaxConcurrency)
	var wait sync.WaitGroup
	for index, job := range jobs {
		index, job := index, job
		wait.Add(1)
		go func() {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			source, err := runDirectCodingTypeScriptFragmentWorker(runtime, modelName, job)
			results[index] = directCodingFragmentResult{blockID: job.block.ID, source: source, err: err}
		}()
	}
	wait.Wait()
	return results
}

func directCodingTypeScriptAvailableDeclarations(
	block assemblyline.TypeScriptBlock,
	declarations map[string]string,
) (string, error) {
	available := make([]string, 0, len(block.Capabilities))
	for _, capability := range block.Capabilities {
		declaration := strings.TrimSpace(declarations[capability])
		if declaration == "" {
			return "", fmt.Errorf("block %s capability %s has no accepted API", block.ID, capability)
		}
		available = append(available, declaration)
	}
	return strings.Join(available, "\n"), nil
}

func directCodingTypeScriptAcceptedDeclarations(
	blueprint assemblyline.TypeScriptBlueprint,
	generated map[string]string,
) (map[string]string, error) {
	declarations := make(map[string]string)
	waves, err := blueprint.BuildWaves()
	if err != nil {
		return nil, err
	}
	for _, wave := range waves {
		for _, ref := range wave {
			if ref.Block.Generated() && strings.TrimSpace(generated[ref.Block.ID]) == "" {
				return nil, fmt.Errorf("generated block %s has no accepted declaration", ref.Block.ID)
			}
			declarations[ref.Block.ID] = strings.TrimSpace(ref.Block.API)
		}
	}
	return declarations, nil
}

func directCodingTypeScriptBlueprintBlock(
	blueprint assemblyline.TypeScriptBlueprint,
	blockID string,
) (assemblyline.TypeScriptBlock, bool) {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return block, true
			}
		}
	}
	return assemblyline.TypeScriptBlock{}, false
}
