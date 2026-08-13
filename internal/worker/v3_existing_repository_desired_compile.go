package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

var errDesiredArtifactUsesExistingDeclaration = errors.New("desired artifact already has exact declaration authority")

func compileExistingRepositoryDesiredGraph(
	authority string,
	partition assemblyline.RequirementPartitionDecision,
	resolutions []existingRepositoryRequirementResolution,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (repositoryfacts.DesiredArtifactGraph, error) {
	if len(directives) != 0 {
		if err := validateDesiredDeletionFeatureCoverage(partition.FeatureQuotes, directives); err != nil {
			return repositoryfacts.DesiredArtifactGraph{}, err
		}
		return compileDesiredArtifactDeletion(
			authority, partition.FeatureQuotes, directives, identities, snapshot, analysis,
		)
	}
	if len(partition.FeatureQuotes) != len(resolutions) {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("desired repository compilation requires one resolution per exact requirement")
	}
	if hasResolvedRepositoryTarget(resolutions) {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("mixed repository modification and file-state transitions are unsupported")
	}
	return compileDesiredArtifactCreation(authority, partition.FeatureQuotes, snapshot, analysis)
}

func validateDesiredDeletionFeatureCoverage(
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
) error {
	for _, quote := range featureQuotes {
		covered := false
		for _, directive := range directives {
			if containsExactArtifactToken(quote, directive.Token) {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf(
				"mixed create, modify, and delete requirements are unsupported; exact requirement %q has no artifact truth binding",
				quote,
			)
		}
	}
	return nil
}

func validateDesiredCreationFeatureCoverage(
	creationQuote string,
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
) error {
	for _, quote := range featureQuotes {
		if quote == creationQuote {
			continue
		}
		covered := false
		for _, directive := range directives {
			if directive.Disposition != assemblyline.ArtifactForbid &&
				containsExactArtifactToken(quote, directive.Token) {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf(
				"mixed create and modify requirements are unsupported; exact requirement %q is unconsumed",
				quote,
			)
		}
	}
	return nil
}

func exactExistingRepositoryGoAnalysis(
	snapshot repositoryfacts.Snapshot,
	analyses []repositoryfacts.Analysis,
) (repositoryfacts.Analysis, error) {
	var found repositoryfacts.Analysis
	for _, analysis := range analyses {
		if analysis.SnapshotID != snapshot.ID || analysis.Adapter.Name != "go" || !analysis.Complete {
			continue
		}
		if found.ID != "" {
			return repositoryfacts.Analysis{}, fmt.Errorf("repository contains multiple complete Go analyses")
		}
		found = analysis
	}
	if found.ID == "" {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository has no complete Go analysis for desired state")
	}
	if err := found.Validate(snapshot); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("desired repository Go analysis: %w", err)
	}
	return found, nil
}

func compileDesiredArtifactCreation(
	authority string,
	featureQuotes []string,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (repositoryfacts.DesiredArtifactGraph, error) {
	signature, err := gofragment.ExtractUniqueNewFunctionSignature(authority)
	if err != nil {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("compile desired new Go declaration: %w", err)
	}
	quote := ""
	for _, candidate := range featureQuotes {
		if !strings.Contains(candidate, signature.Source) {
			continue
		}
		if quote != "" {
			return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("new Go signature belongs to multiple exact requirements")
		}
		quote = candidate
	}
	if quote == "" {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("new Go signature is not contained in one exact requirement")
	}
	for _, symbol := range analysis.Symbols {
		if symbol.Name != signature.Name {
			continue
		}
		if symbol.Signature == signature.Canonical {
			return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf(
				"Go declaration %q already exists and requires ordinary bounded modification: %w",
				signature.Name, errDesiredArtifactUsesExistingDeclaration,
			)
		}
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("Go declaration name %q already exists under a different signature", signature.Name)
	}
	placement, err := repositoryfacts.UniqueGoPackagePlacement(snapshot, analysis)
	if err != nil {
		return repositoryfacts.DesiredArtifactGraph{}, err
	}
	return repositoryfacts.NewDesiredArtifactGraph(
		snapshot, analysis, desiredGraphOwner(authority),
		[]repositoryfacts.DesiredGoArtifact{{
			RequirementQuote: quote, PackageArtifactID: placement.ArtifactID,
			Signature: signature.Canonical, MustExist: true,
		}},
	)
}

func explicitMissingGoArtifactCandidate(
	authority string,
	featureQuotes []string,
	analysis repositoryfacts.Analysis,
) (gofragment.NewFunctionSignature, string, bool, error) {
	signature, err := gofragment.ExtractUniqueNewFunctionSignature(authority)
	if err != nil {
		if errors.Is(err, gofragment.ErrNoExplicitFunctionSignature) {
			return gofragment.NewFunctionSignature{}, "", false, nil
		}
		return gofragment.NewFunctionSignature{}, "", false, err
	}
	quote := ""
	for _, candidate := range featureQuotes {
		if !strings.Contains(candidate, signature.Source) {
			continue
		}
		if quote != "" {
			return gofragment.NewFunctionSignature{}, "", false, fmt.Errorf(
				"new Go signature belongs to multiple exact requirements",
			)
		}
		quote = candidate
	}
	if quote == "" {
		return gofragment.NewFunctionSignature{}, "", false, fmt.Errorf(
			"new Go signature is not contained in one exact requirement",
		)
	}
	for _, symbol := range analysis.Symbols {
		if symbol.Name != signature.Name {
			continue
		}
		if symbol.Signature == signature.Canonical {
			return gofragment.NewFunctionSignature{}, "", false, nil
		}
		return gofragment.NewFunctionSignature{}, "", false, fmt.Errorf(
			"Go declaration name %q already exists under a different signature", signature.Name,
		)
	}
	return signature, quote, true, nil
}

func compileDesiredArtifactDeletion(
	authority string,
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (repositoryfacts.DesiredArtifactGraph, error) {
	identityValues := make(map[string]string, len(identities))
	for _, identity := range identities {
		identityValues[identity.Token] = identity.Value
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.Path] = file
	}
	artifacts := make([]repositoryfacts.DesiredGoArtifact, 0, len(directives))
	for _, directive := range directives {
		if directive.Disposition != assemblyline.ArtifactForbid {
			continue
		}
		requirementQuote, err := exactArtifactAbsenceRequirementQuote(
			directive.Token, featureQuotes,
		)
		if err != nil {
			return repositoryfacts.DesiredArtifactGraph{}, err
		}
		value, exists := identityValues[directive.Token]
		if !exists {
			return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("desired absent artifact %q has no code-held identity", directive.Token)
		}
		file, exists := files[path.Clean(value)]
		if !exists || file.Path != value {
			return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("desired absent artifact %q is not an exact indexed source member", directive.Token)
		}
		symbolIDs := make([]string, 0)
		for _, symbol := range analysis.Symbols {
			if symbol.FileID == file.ID {
				symbolIDs = append(symbolIDs, symbol.ID)
			}
		}
		sort.Strings(symbolIDs)
		if len(symbolIDs) == 0 {
			return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("desired absent artifact %q has no indexed declarations", directive.Token)
		}
		placement, err := repositoryfacts.GoPackagePlacementForSymbols(snapshot, analysis, symbolIDs)
		if err != nil {
			return repositoryfacts.DesiredArtifactGraph{}, err
		}
		artifacts = append(artifacts, repositoryfacts.DesiredGoArtifact{
			RequirementQuote: requirementQuote, PackageArtifactID: placement.ArtifactID,
			MustExist: false, ExistingSymbolIDs: symbolIDs,
		})
	}
	if len(artifacts) == 0 {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf("desired repository deletion requires one explicit absent artifact")
	}
	return repositoryfacts.NewDesiredArtifactGraph(
		snapshot, analysis, desiredGraphOwner(authority), artifacts,
	)
}

func exactArtifactAbsenceRequirementQuote(token string, featureQuotes []string) (string, error) {
	quote := ""
	for _, candidate := range featureQuotes {
		if !containsExactArtifactToken(candidate, token) {
			continue
		}
		if quote != "" {
			return "", fmt.Errorf(
				"desired absent artifact %q belongs to multiple exact requirements", token,
			)
		}
		quote = candidate
	}
	if quote == "" {
		return "", fmt.Errorf(
			"desired absent artifact %q has no exact absence requirement", token,
		)
	}
	return quote, nil
}

func containsExactArtifactToken(value, token string) bool {
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], token)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(token)
		leftExact := start == 0 || !artifactTokenByte(value[start-1])
		rightExact := end == len(value) || !artifactTokenByte(value[end])
		if leftExact && rightExact {
			return true
		}
		offset = start + 1
	}
	return false
}

func artifactTokenByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' ||
		value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func hasResolvedRepositoryTarget(resolutions []existingRepositoryRequirementResolution) bool {
	for _, resolution := range resolutions {
		if len(resolution.Surface.Targets) != 0 {
			return true
		}
	}
	return false
}

func desiredGraphOwner(authority string) string {
	// The hash-based graph ID binds the full exact owner; this bounded prefix is
	// retained only as readable local authority and never enters a model prompt.
	digest := sha256.Sum256([]byte(authority))
	return "objective:" + hex.EncodeToString(digest[:])
}
