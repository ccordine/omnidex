package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func buildObjectiveRepositoryEvidence(
	ctx context.Context,
	builder repositoryEvidenceBuilder,
	projectID int64,
	analyses []repositoryfacts.Analysis,
	codeOwnedQuery string,
) (repositoryretrieval.EvidencePack, error) {
	if ctx == nil || builder == nil || projectID < 1 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository evidence build requires context, builder, and project authority")
	}
	if err := ctx.Err(); err != nil {
		return repositoryretrieval.EvidencePack{}, err
	}
	ordered := append([]repositoryfacts.Analysis(nil), analyses...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ID < ordered[right].ID })
	packs := make([]repositoryretrieval.EvidencePack, 0, len(ordered))
	for _, analysis := range ordered {
		if err := ctx.Err(); err != nil {
			return repositoryretrieval.EvidencePack{}, err
		}
		request, err := newExistingRepositoryEvidenceRequest(projectID, analysis.ID, codeOwnedQuery)
		if err != nil {
			return repositoryretrieval.EvidencePack{}, err
		}
		pack, err := builder.Build(ctx, request)
		if errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) {
			continue
		}
		if err != nil {
			return repositoryretrieval.EvidencePack{}, err
		}
		if err := pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts,
			codeOwnedQuery,
		); err != nil {
			return repositoryretrieval.EvidencePack{}, fmt.Errorf(
				"repository evidence analysis %q returned invalid evidence: %w",
				analysis.ID,
				err,
			)
		}
		packs = append(packs, pack)
	}
	if len(packs) == 0 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"%w: code-owned lexical neighborhood for %q matched no complete analysis",
			repositoryretrieval.ErrInsufficientEvidence, codeOwnedQuery,
		)
	}
	if len(packs) == 1 {
		return packs[0], nil
	}
	return mergeObjectiveRepositoryEvidencePacks(packs, codeOwnedQuery)
}

func mergeObjectiveRepositoryEvidencePacks(
	packs []repositoryretrieval.EvidencePack,
	codeOwnedQuery string,
) (repositoryretrieval.EvidencePack, error) {
	if len(packs) < 2 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("polyglot repository evidence merge requires at least two packs")
	}
	ordered := append([]repositoryretrieval.EvidencePack(nil), packs...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].AnalysisID < ordered[right].AnalysisID
	})
	snapshotID := ordered[0].SnapshotID
	for _, pack := range ordered {
		if err := pack.ValidateForRequest(repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery); err != nil {
			return repositoryretrieval.EvidencePack{}, err
		}
		if pack.SnapshotID != snapshotID {
			return repositoryretrieval.EvidencePack{}, fmt.Errorf("polyglot repository evidence must share one snapshot")
		}
	}
	merged := repositoryretrieval.EvidencePack{
		Schema:           repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID:       snapshotID,
		AnalysisID:       objectiveRepositoryAnalysisSetID(ordered),
		Operation:        repositoryretrieval.OperationSemanticExcerpts,
		QueryBinding:     ordered[0].QueryBinding,
		Symbols:          []repositoryretrieval.EvidenceSymbol{},
		Relations:        []repositoryretrieval.EvidenceRelation{},
		SourceOmissions:  []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{},
		MaxBytes:         64 * 1024,
	}
	seen := make(map[objectiveRepositorySymbolIdentity]string)
	seenRelations := make(map[objectiveRepositoryRelationIdentity]struct{})
	mergedSymbolNames := make(map[string]string)
	projectedBytes := 0
	for analysisIndex, pack := range ordered {
		if len(pack.SourceOmissions) > 0 || len(pack.OmittedSymbolIDs) > 0 || pack.OmittedEdges > 0 {
			return repositoryretrieval.EvidencePack{}, fmt.Errorf(
				"polyglot repository evidence pack %q is incomplete: source_omissions=%d omitted_symbols=%d omitted_edges=%d",
				pack.AnalysisID, len(pack.SourceOmissions), len(pack.OmittedSymbolIDs), pack.OmittedEdges,
			)
		}
		symbols := append([]repositoryretrieval.EvidenceSymbol(nil), pack.Symbols...)
		sort.Slice(symbols, func(left, right int) bool { return symbols[left].ID < symbols[right].ID })
		remapped := make(map[string]string, len(symbols))
		for symbolIndex, symbol := range symbols {
			originalID := symbol.ID
			identity := objectiveRepositorySymbolIdentity{
				kind: symbol.Kind, name: symbol.Name, signature: symbol.Signature,
				sourceSHA256: symbol.SourceSHA256, source: symbol.Source,
			}
			if existingID, duplicate := seen[identity]; duplicate {
				remapped[originalID] = existingID
				continue
			}
			bytes, err := validateRepositoryEvidenceSymbolBounds(pack.ID, symbol)
			if err != nil {
				return repositoryretrieval.EvidencePack{}, err
			}
			if len(merged.Symbols)+len(merged.Relations) >= maxObjectiveRepositoryEvidenceCapsules {
				return repositoryretrieval.EvidencePack{}, fmt.Errorf(
					"polyglot repository evidence exceeds the global %d-fact limit",
					maxObjectiveRepositoryEvidenceCapsules,
				)
			}
			if projectedBytes+len("R00")+bytes > maxObjectiveRepositoryEvidenceTotalBytes {
				return repositoryretrieval.EvidencePack{}, fmt.Errorf(
					"polyglot repository evidence exceeds the global %d-byte projection limit",
					maxObjectiveRepositoryEvidenceTotalBytes,
				)
			}
			symbol.ID = objectiveRepositoryMergedSymbolID(
				analysisIndex,
				symbolIndex,
				pack.AnalysisID,
				originalID,
			)
			merged.Symbols = append(merged.Symbols, symbol)
			seen[identity] = symbol.ID
			remapped[originalID] = symbol.ID
			mergedSymbolNames[symbol.ID] = symbol.Name
			projectedBytes += len("R00") + bytes
		}
		relations := append([]repositoryretrieval.EvidenceRelation(nil), pack.Relations...)
		sort.Slice(relations, func(left, right int) bool { return relations[left].ID < relations[right].ID })
		for relationIndex, relation := range relations {
			fromID, fromExists := remapped[relation.FromID]
			toID, toExists := remapped[relation.ToID]
			if !fromExists || !toExists {
				return repositoryretrieval.EvidencePack{}, fmt.Errorf(
					"polyglot repository relation %q lost a bounded endpoint", relation.ID,
				)
			}
			relation.FromID, relation.ToID = fromID, toID
			identity := objectiveRepositoryRelationIdentity{
				fromID: fromID, toID: toID, kind: relation.Kind,
				origin: relation.Origin, confidence: relation.Confidence,
			}
			if _, duplicate := seenRelations[identity]; duplicate {
				continue
			}
			if len(merged.Symbols)+len(merged.Relations) >= maxObjectiveRepositoryEvidenceCapsules {
				return repositoryretrieval.EvidencePack{}, fmt.Errorf(
					"polyglot repository evidence exceeds the global %d-fact limit",
					maxObjectiveRepositoryEvidenceCapsules,
				)
			}
			relation.ID = objectiveRepositoryMergedRelationID(
				analysisIndex, relationIndex, pack.AnalysisID, relation.ID,
			)
			text, err := repositoryRelationEvidenceText(relation, mergedSymbolNames)
			if err != nil {
				return repositoryretrieval.EvidencePack{}, err
			}
			if projectedBytes+len("R00")+len(text) > maxObjectiveRepositoryEvidenceTotalBytes {
				return repositoryretrieval.EvidencePack{}, fmt.Errorf(
					"polyglot repository evidence exceeds the global %d-byte projection limit",
					maxObjectiveRepositoryEvidenceTotalBytes,
				)
			}
			merged.Relations = append(merged.Relations, relation)
			seenRelations[identity] = struct{}{}
			projectedBytes += len("R00") + len(text)
		}
	}
	if len(merged.Symbols) == 0 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("polyglot repository evidence merge produced no bounded symbols")
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&merged); err != nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("finalize polyglot repository evidence: %w", err)
	}
	return merged, nil
}

type objectiveRepositorySymbolIdentity struct {
	kind, name, signature, sourceSHA256, source string
}

type objectiveRepositoryRelationIdentity struct {
	fromID, toID, kind, origin string
	confidence                 float64
}

func objectiveRepositoryAnalysisSetID(packs []repositoryretrieval.EvidencePack) string {
	hash := sha256.New()
	for _, pack := range packs {
		_, _ = hash.Write([]byte(pack.AnalysisID))
		_, _ = hash.Write([]byte{0})
	}
	return "analysis_set_" + hex.EncodeToString(hash.Sum(nil))
}

func objectiveRepositoryMergedSymbolID(
	analysisIndex int,
	symbolIndex int,
	analysisID string,
	symbolID string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(analysisID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(symbolID))
	return fmt.Sprintf(
		"symbol_%08d_%08d_%s",
		analysisIndex,
		symbolIndex,
		hex.EncodeToString(hash.Sum(nil)),
	)
}

func objectiveRepositoryMergedRelationID(
	analysisIndex int,
	relationIndex int,
	analysisID string,
	relationID string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(analysisID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(relationID))
	return fmt.Sprintf(
		"relation_%08d_%08d_%s",
		analysisIndex,
		relationIndex,
		hex.EncodeToString(hash.Sum(nil)),
	)
}
