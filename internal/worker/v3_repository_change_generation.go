package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
)

func (session *directCodingSession) generateExistingRepositoryChangeCandidates(
	contract repositoryfacts.ChangeContract,
	baseline *verifiedRepositoryBaseline,
) (map[string]string, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return nil, fmt.Errorf("repository change generation requires one immutable index")
	}
	analysis, err := exactRepositoryChangeAnalysis(session.repositoryIndex.Analyses, contract.AnalysisID)
	if err != nil {
		return nil, err
	}
	if err := contract.Validate(session.repositoryIndex.Snapshot, analysis); err != nil {
		return nil, fmt.Errorf("validate repository change contract before generation: %w", err)
	}
	commands, err := existingRepositoryGoVerificationCommands(
		session.repositoryIndex.Snapshot, analysis, contract,
	)
	if err != nil {
		return nil, err
	}
	if err := baseline.RequireAuthority(
		session.repositoryIndex.Snapshot.ID, contract.ID, commands,
	); err != nil {
		return nil, fmt.Errorf("authorize repository generation from clean baseline: %w", err)
	}
	modelName, err := session.workerModel(station.CodingFragment)
	if err != nil {
		return nil, err
	}
	runtime := directCodingWorkerRuntime(session)
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	targets := append([]repositoryfacts.ChangeTarget(nil), contract.Targets...)
	sort.Slice(targets, func(left, right int) bool { return targets[left].SymbolID < targets[right].SymbolID })
	profiles := make(map[string]directCodingProjectVersionProfile, len(targets))
	qualifiedProfiles := make(map[string]struct{})
	for _, target := range targets {
		symbol, exists := symbols[target.SymbolID]
		if !exists {
			return nil, fmt.Errorf("repository change target %q disappeared from exact analysis", target.SymbolID)
		}
		targetPath, err := repositorySnapshotFilePath(
			session.repositoryIndex.Snapshot, symbol.FileID,
		)
		if err != nil {
			return nil, err
		}
		profile, err := repositoryGoVersionProfile(session.repositoryIndex.Snapshot, targetPath)
		if err != nil {
			return nil, err
		}
		if _, qualified := qualifiedProfiles[profile.ID]; !qualified {
			if err := validateDirectCodingVersionProfileRuntime(
				profile, directCodingSessionVersionProbe(session.runtime.ctx, session.root),
			); err != nil {
				return nil, err
			}
			qualifiedProfiles[profile.ID] = struct{}{}
		}
		profiles[target.SymbolID] = profile
	}
	candidates := make(map[string]string, len(targets))
	for _, target := range targets {
		if err := session.requireCurrentRepositoryAuthority("fragment projection"); err != nil {
			return nil, err
		}
		symbol := symbols[target.SymbolID]
		profile := profiles[target.SymbolID]
		span, err := repositoryfacts.ReadExactSymbolSpan(
			session.repositoryIndex.Snapshot, symbol, int(target.EndByte-target.StartByte),
		)
		if err != nil {
			return nil, err
		}
		input, err := existingRepositoryGoModificationInput(
			target, span.Content, profile.SourceDialect,
		)
		if err != nil {
			return nil, err
		}
		candidate, err := runDirectCodingGoFragmentModificationWorker(
			runtime, modelName, directCodingGoModificationJob{Subject: target.SymbolID, Input: input},
		)
		if err != nil {
			return nil, err
		}
		candidates[target.SymbolID] = candidate
	}
	return candidates, nil
}

func existingRepositoryGoModificationInput(
	target repositoryfacts.ChangeTarget,
	current string,
	dialect string,
) (assemblyline.FragmentModificationInput, error) {
	digest := sha256.Sum256([]byte(current))
	if hex.EncodeToString(digest[:]) != target.ExpectedDeclarationSHA256 {
		return assemblyline.FragmentModificationInput{}, fmt.Errorf(
			"repository change target %q declaration hash is stale", target.SymbolID,
		)
	}
	capabilities := make([]string, len(target.DirectCapabilities))
	for index, capability := range target.DirectCapabilities {
		capabilities[index] = capability.Signature
	}
	permitted := target.PermittedCapabilitySymbols()
	input := assemblyline.FragmentModificationInput{
		Language: "go", Dialect: dialect, Signature: target.Signature,
		CurrentDeclaration: current, RequirementQuote: target.RequirementQuote,
		Capabilities: capabilities, PermittedSymbols: permitted,
	}
	job, err := assemblyline.NewFragmentModificationJob(input)
	if err != nil {
		return assemblyline.FragmentModificationInput{}, fmt.Errorf(
			"build repository change target %q model boundary: %w", target.SymbolID, err,
		)
	}
	if job.Kind != assemblyline.WorkFragmentModification {
		return assemblyline.FragmentModificationInput{}, fmt.Errorf("repository change target produced unexpected work kind %q", job.Kind)
	}
	return input, nil
}

func exactRepositoryChangeAnalysis(
	analyses []repositoryfacts.Analysis,
	id string,
) (repositoryfacts.Analysis, error) {
	var found repositoryfacts.Analysis
	for _, analysis := range analyses {
		if analysis.ID != id {
			continue
		}
		if found.ID != "" {
			return repositoryfacts.Analysis{}, fmt.Errorf("repository index contains duplicate analysis %q", id)
		}
		found = analysis
	}
	if found.ID == "" {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis %q is absent from the immutable index", id)
	}
	return found, nil
}
