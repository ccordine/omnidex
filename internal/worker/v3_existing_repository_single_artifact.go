package worker

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/station"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

// explicitPlainTextArtifactPaths performs only lexical and adapter validation.
// It does not infer that the user wants to create, modify, or delete a path.
func explicitPlainTextArtifactPaths(authority string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, token := range modelcontext.LexicalPathTokens(authority) {
		adapter, kind, recognized, err := recognizeDirectCodingArtifactAdapterForPath(token.Value)
		if err != nil {
			return nil, err
		}
		if !recognized || adapter.ID != assemblyline.PlainTextAdapterID {
			continue
		}
		if kind != assemblyline.TargetArtifactImplementation || adapter.ComposeDocument == nil {
			return nil, fmt.Errorf(
				"plain-text artifact token %q lacks its executable implementation composer",
				token.Value,
			)
		}
		seen[token.Value] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for artifactPath := range seen {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	return paths, nil
}

// tryExistingRepositorySinglePlainTextCreation opens one narrow semantic gap
// only when code has already established exactly one explicit, absent,
// adapter-recognized plain-text path. The returned truth is semantic data; code
// owns the resulting route and every operation.
func (session *directCodingSession) tryExistingRepositorySinglePlainTextCreation(
	authority string,
	redactedAuthority string,
	identities []assemblyline.ArtifactIdentity,
	explicitPaths []string,
) (summary string, handled bool, resultErr error) {
	if len(explicitPaths) != 1 || session == nil || session.repositoryIndex == nil {
		return "", false, nil
	}
	artifactPath := explicitPaths[0]
	for _, file := range session.repositoryIndex.Snapshot.Files {
		if file.Path == artifactPath {
			return "", false, nil
		}
	}
	if err := validatePlainTextPathBlindValue(redactedAuthority); err != nil {
		return "", true, err
	}
	if !artifactIdentityBoundToPath(identities, artifactPath) {
		return "", true, fmt.Errorf(
			"explicit plain-text target %q lacks one opaque model-context identity",
			artifactPath,
		)
	}
	modelName, err := session.workerModel(station.CodingKnownArtifactTruth)
	if err != nil {
		return "", true, err
	}
	runtime := directCodingWorkerRuntime(session)
	runtime.MaxAttempts = 1
	decision, err := classifyKnownArtifactTruth(
		runtime, modelName,
		assemblyline.KnownArtifactTruthInput{RequirementQuote: redactedAuthority}, identities,
	)
	if err != nil {
		return "", true, err
	}
	if decision.Truth != assemblyline.OnePlainTextArtifactMustExist {
		return "", false, nil
	}
	summary, err = session.runExistingRepositorySingleArtifactCreation(
		authority, redactedAuthority, artifactPath,
	)
	return summary, true, err
}

func artifactIdentityBoundToPath(
	identities []assemblyline.ArtifactIdentity,
	artifactPath string,
) bool {
	found := 0
	for _, identity := range identities {
		if identity.Value == artifactPath {
			found++
		}
	}
	return found == 1
}

func (session *directCodingSession) runExistingRepositorySingleArtifactCreation(
	authority string,
	pathRedactedRequirement string,
	artifactPath string,
) (string, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("single-artifact creation requires one active indexed session")
	}
	adapter, kind, err := directCodingArtifactAdapterForPath(artifactPath)
	if err != nil {
		return "", err
	}
	if adapter.ID != assemblyline.PlainTextAdapterID ||
		kind != assemblyline.TargetArtifactImplementation || adapter.ComposeDocument == nil {
		return "", fmt.Errorf(
			"single-artifact creation path %q is outside the code-selected plain-text adapter",
			artifactPath,
		)
	}
	task, err := assemblyline.FreezePlainTextArtifactTask(authority, pathRedactedRequirement)
	if err != nil {
		return "", err
	}
	targetInput, target, source, err := session.projectSinglePlainTextTarget(task, artifactPath, adapter)
	if err != nil {
		return "", err
	}
	transitions, err := assemblyline.DiffTargetTree(targetInput, target)
	if err != nil {
		return "", fmt.Errorf("derive single plain-text target transitions: %w", err)
	}
	if err := validateSinglePlainTextCreateTransitions(transitions, artifactPath); err != nil {
		return "", err
	}
	if err := repositoryfacts.RequireGitPathVisible(
		session.runtime.ctx, session.repositoryIndex.Snapshot.Root, artifactPath,
	); err != nil {
		return "", err
	}
	ownerID := singleArtifactMutationOwnerID(task.SHA256, session.repositoryIndex.Snapshot.ID)
	if err := validateSingleArtifactCreationTarget(
		session.runtime.ctx, source, ownerID, target, adapter,
	); err != nil {
		return "", err
	}
	coverage, err := assemblyline.NewPlainTextArtifactCoverage(task, target)
	if err != nil {
		return "", err
	}
	blueprint, err := assemblyline.CompilePlainTextArtifactBlueprint(task, target, coverage)
	if err != nil {
		return "", err
	}
	if err := session.bindDirectCodingTargetTreePathProvenance(target); err != nil {
		return "", fmt.Errorf("bind single-artifact target provenance: %w", err)
	}
	sourceText, err := session.generateSinglePlainTextArtifact(blueprint, adapter)
	if err != nil {
		return "", err
	}
	return session.applySinglePlainTextArtifact(
		task.SHA256, artifactPath, []byte(sourceText), adapter,
	)
}

func (session *directCodingSession) projectSinglePlainTextTarget(
	task assemblyline.FrozenPlainTextArtifactTask,
	artifactPath string,
	adapter directCodingArtifactAdapter,
) (assemblyline.TargetTreeInput, assemblyline.TargetTree, workspacefacts.Snapshot, error) {
	var zeroInput assemblyline.TargetTreeInput
	var zeroTarget assemblyline.TargetTree
	var zeroSource workspacefacts.Snapshot
	if err := task.Validate(); err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	if adapter.ID != assemblyline.PlainTextAdapterID {
		return zeroInput, zeroTarget, zeroSource, fmt.Errorf("plain-text target requires its selected adapter")
	}
	existingPaths, existingDirs, err := snapshotDirectCodingTargetTreePaths(session.root)
	if err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	input := assemblyline.TargetTreeInput{
		Objective:        task.Requirement,
		TechnicalContext: "Code-selected plain_text adapter: exactly one UTF-8 .txt, .gitignore, or .dockerignore implementation document with LF line endings and one terminal LF.",
		Constraints:      assemblyline.TargetTreeConstraints{ExactPathCount: 1},
		ExistingPaths:    existingPaths, ReusablePaths: []string{},
		ReservedPaths: []string{}, ExistingDirs: existingDirs,
	}
	if err := input.Validate(); err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	target := assemblyline.TargetTree{
		StackID:          assemblyline.PlainTextArtifactStackID,
		VersionProfileID: assemblyline.PlainTextArtifactProfileID,
		Paths:            []string{artifactPath},
	}
	if err := assemblyline.ValidateTargetTreeConstraints(input.Constraints, target); err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	if err := assemblyline.ValidateTargetTreeExistingDirectories(input.ExistingDirs, target); err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	source, err := workspacefacts.FromRepositorySnapshot(session.repositoryIndex.Snapshot)
	if err != nil {
		return zeroInput, zeroTarget, zeroSource, err
	}
	return input, target, source, nil
}

func validateSinglePlainTextCreateTransitions(
	transitions []assemblyline.TargetTreeTransition,
	artifactPath string,
) error {
	creates := 0
	for _, transition := range transitions {
		switch transition.Kind {
		case assemblyline.TargetTreeEnsureDirectory:
			continue
		case assemblyline.TargetTreeCreate:
			if transition.Path != artifactPath {
				return fmt.Errorf("plain-text target transition created an unrelated path %q", transition.Path)
			}
			creates++
		case assemblyline.TargetTreeReconcile:
			return fmt.Errorf("plain-text creation target %q already exists", transition.Path)
		default:
			return fmt.Errorf("plain-text target transition %q is unsupported", transition.Kind)
		}
	}
	if creates != 1 {
		return fmt.Errorf("plain-text target requires exactly one code-owned create transition")
	}
	return nil
}

func validateSingleArtifactCreationTarget(
	ctx context.Context,
	source workspacefacts.Snapshot,
	ownerID string,
	target assemblyline.TargetTree,
	selectedAdapter directCodingArtifactAdapter,
) error {
	if target.StackID != assemblyline.PlainTextArtifactStackID ||
		target.VersionProfileID != assemblyline.PlainTextArtifactProfileID {
		return fmt.Errorf("single-artifact target differs from its code-selected stack authority")
	}
	if err := assemblyline.ValidateTargetTreeConstraints(
		assemblyline.TargetTreeConstraints{ExactPathCount: 1}, target,
	); err != nil {
		return err
	}
	artifactPath := target.Paths[0]
	adapter, kind, err := directCodingArtifactAdapterForPath(artifactPath)
	if err != nil {
		return err
	}
	if selectedAdapter.ID != assemblyline.PlainTextAdapterID || adapter.ID != selectedAdapter.ID ||
		kind != assemblyline.TargetArtifactImplementation || adapter.ComposeDocument == nil {
		return fmt.Errorf("single-artifact target %q differs from its selected executable composer", artifactPath)
	}
	_, err = workspacefacts.PlanMutation(ctx, source, ownerID, []workspacefacts.DesiredFileState{{
		Path: artifactPath, Present: true, Content: []byte("pending\n"), Mode: 0o644,
	}})
	if err != nil {
		return fmt.Errorf("single-artifact target %q is not one safe absent workspace leaf: %w", artifactPath, err)
	}
	return nil
}

func singleArtifactMutationOwnerID(requirement, snapshotID string) string {
	return "plain_text_" + directCodingDigest(requirement+"\x00"+snapshotID)
}
