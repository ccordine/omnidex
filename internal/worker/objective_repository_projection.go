package worker

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const (
	maxObjectiveRepositoryEvidenceCapsules   = 8
	maxObjectiveRepositoryEvidenceTextBytes  = 2 * 1024
	maxObjectiveRepositoryEvidenceTotalBytes = 8 * 1024
)

func repositoryEvidenceCapsules(
	pack repositoryretrieval.EvidencePack,
	provenance assemblyline.ArtifactIdentityProvenance,
) ([]objectiveEvidence, error) {
	if err := validateRepositoryEvidenceProjectionBounds(pack); err != nil {
		return nil, err
	}
	symbols := append([]repositoryretrieval.EvidenceSymbol(nil), pack.Symbols...)
	sort.Slice(symbols, func(left, right int) bool { return symbols[left].ID < symbols[right].ID })
	relations := append([]repositoryretrieval.EvidenceRelation(nil), pack.Relations...)
	sort.Slice(relations, func(left, right int) bool { return relations[left].ID < relations[right].ID })
	capsules := make([]objectiveEvidence, 0, len(symbols)+len(relations))
	seenText := make(map[string]struct{}, len(symbols)+len(relations))
	symbolNames := make(map[string]string, len(symbols))
	for _, symbol := range symbols {
		selectionText := strings.TrimSpace(strings.Join(
			[]string{symbol.Kind, symbol.Name, symbol.Signature}, "\n",
		))
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			"repository symbol selection projection", provenance, selectionText,
		); err != nil {
			return nil, fmt.Errorf("repository evidence symbol %q: %w", symbol.ID, err)
		}
		source, err := redactRepositorySourcePathIdentities(symbol.Source, provenance)
		if err != nil {
			return nil, fmt.Errorf("repository evidence symbol %q: %w", symbol.ID, err)
		}
		text := selectionText
		if strings.TrimSpace(source) != "" {
			text += "\n" + source
		}
		item, err := appendRepositoryObjectiveEvidence(
			&capsules, seenText, text, "repository_symbol", pack.ID+"#"+symbol.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("repository evidence symbol %q: %w", symbol.ID, err)
		}
		item.SelectionText = selectionText
		item.SourceSHA256 = symbol.SourceSHA256
		capsules[len(capsules)-1] = item
		symbolNames[symbol.ID] = symbol.Name
	}
	for _, relation := range relations {
		text, err := repositoryRelationEvidenceText(relation, symbolNames)
		if err != nil {
			return nil, err
		}
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			"repository relation selection projection", provenance, text,
		); err != nil {
			return nil, fmt.Errorf("repository evidence relation %q: %w", relation.ID, err)
		}
		item, err := appendRepositoryObjectiveEvidence(
			&capsules, seenText, text, "repository_relation", pack.ID+"#"+relation.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("repository evidence relation %q: %w", relation.ID, err)
		}
		item.SelectionText = text
		capsules[len(capsules)-1] = item
	}
	return capsules, nil
}

func redactRepositorySourcePathIdentities(
	source string,
	provenance assemblyline.ArtifactIdentityProvenance,
) (string, error) {
	identities := modelcontext.SourcePathIdentities(source, provenance)
	if len(identities) == 0 {
		if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"repository source projection", provenance, source,
		); err != nil {
			return "", err
		}
		return source, nil
	}
	var projected strings.Builder
	previous := 0
	for _, identity := range identities {
		if identity.Start < previous || identity.End < identity.Start || identity.End > len(source) {
			return "", fmt.Errorf("repository source path projection contains overlapping evidence")
		}
		projected.WriteString(source[previous:identity.Start])
		projected.WriteString("ARTIFACT_REF")
		previous = identity.End
	}
	projected.WriteString(source[previous:])
	result := projected.String()
	if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"repository source projection", provenance, result,
	); err != nil {
		return "", err
	}
	return result, nil
}

func repositoryRelationEvidenceText(
	relation repositoryretrieval.EvidenceRelation,
	symbolNames map[string]string,
) (string, error) {
	fromName, fromExists := symbolNames[relation.FromID]
	toName, toExists := symbolNames[relation.ToID]
	if !fromExists || !toExists {
		return "", fmt.Errorf("repository evidence relation %q lacks projected endpoints", relation.ID)
	}
	return fromName + " " + relation.Kind + " " + toName, nil
}

func appendRepositoryObjectiveEvidence(
	capsules *[]objectiveEvidence,
	seenText map[string]struct{},
	text string,
	sourceType string,
	sourceRef string,
) (objectiveEvidence, error) {
	if capsules == nil || len(*capsules) >= maxObjectiveRepositoryEvidenceCapsules {
		return objectiveEvidence{}, fmt.Errorf("repository evidence exceeds the %d-capsule global projection limit", maxObjectiveRepositoryEvidenceCapsules)
	}
	if _, duplicate := seenText[text]; duplicate {
		return objectiveEvidence{}, fmt.Errorf("duplicates projected evidence")
	}
	total := 0
	for _, item := range *capsules {
		total += len(item.Capsule.ID) + len(item.Capsule.Text)
	}
	if total+len("R00")+len(text) > maxObjectiveRepositoryEvidenceTotalBytes {
		return objectiveEvidence{}, fmt.Errorf("repository evidence exceeds the %d-byte global projection limit", maxObjectiveRepositoryEvidenceTotalBytes)
	}
	item, err := newObjectiveEvidence(
		fmt.Sprintf("R%02d", len(*capsules)+1), text, sourceType, sourceRef,
	)
	if err != nil {
		return objectiveEvidence{}, err
	}
	seenText[text] = struct{}{}
	*capsules = append(*capsules, item)
	return item, nil
}

func validateRepositoryEvidenceProjectionBounds(pack repositoryretrieval.EvidencePack) error {
	if len(pack.SourceOmissions) > 0 || len(pack.OmittedSymbolIDs) > 0 || pack.OmittedEdges > 0 {
		return fmt.Errorf(
			"repository-read evidence is incomplete: source_omissions=%d omitted_symbols=%d omitted_edges=%d",
			len(pack.SourceOmissions), len(pack.OmittedSymbolIDs), pack.OmittedEdges,
		)
	}
	if len(pack.Symbols)+len(pack.Relations) > maxObjectiveRepositoryEvidenceCapsules {
		return fmt.Errorf(
			"repository-read objective has %d candidate facts and exceeds its global %d-capsule projection",
			len(pack.Symbols)+len(pack.Relations), maxObjectiveRepositoryEvidenceCapsules,
		)
	}
	if len(pack.Symbols) == 0 {
		return fmt.Errorf("repository-read objective produced no evidence capsules")
	}
	if err := validateObjectiveEvidenceLine(
		"repository pack ID", pack.ID, maxObjectiveEvidenceSourceRefBytes-2,
	); err != nil {
		return err
	}
	total := 0
	symbolNames := make(map[string]string, len(pack.Symbols))
	for _, symbol := range pack.Symbols {
		projectedBytes, err := validateRepositoryEvidenceSymbolBounds(pack.ID, symbol)
		if err != nil {
			return err
		}
		total += len("R00") + projectedBytes
		symbolNames[symbol.ID] = symbol.Name
		if total > maxObjectiveRepositoryEvidenceTotalBytes {
			return fmt.Errorf(
				"repository evidence exceeds the %d-byte total projection limit",
				maxObjectiveRepositoryEvidenceTotalBytes,
			)
		}
	}
	for _, relation := range pack.Relations {
		if err := validateObjectiveEvidenceLine(
			"repository relation ID", relation.ID, maxObjectiveEvidenceSourceRefBytes-2,
		); err != nil {
			return err
		}
		if len(pack.ID)+1+len(relation.ID) > maxObjectiveEvidenceSourceRefBytes {
			return fmt.Errorf("repository relation provenance reference exceeds %d bytes", maxObjectiveEvidenceSourceRefBytes)
		}
		if err := validateObjectiveEvidenceLine("repository relation kind", relation.Kind, 128); err != nil {
			return err
		}
		if err := validateObjectiveEvidenceLine("repository relation origin", relation.Origin, 128); err != nil {
			return err
		}
		if math.IsNaN(relation.Confidence) || math.IsInf(relation.Confidence, 0) ||
			relation.Confidence < 0 || relation.Confidence > 1 {
			return fmt.Errorf("repository evidence relation %q has invalid confidence", relation.ID)
		}
		text, err := repositoryRelationEvidenceText(relation, symbolNames)
		if err != nil {
			return err
		}
		if len(text) > maxObjectiveRepositoryEvidenceTextBytes {
			return fmt.Errorf(
				"repository evidence relation %q exceeds the %d-byte capsule limit before projection",
				relation.ID, maxObjectiveRepositoryEvidenceTextBytes,
			)
		}
		total += len("R00") + len(text)
		if total > maxObjectiveRepositoryEvidenceTotalBytes {
			return fmt.Errorf(
				"repository evidence exceeds the %d-byte total projection limit",
				maxObjectiveRepositoryEvidenceTotalBytes,
			)
		}
	}
	return nil
}

func validateRepositoryEvidenceSymbolBounds(
	packID string,
	symbol repositoryretrieval.EvidenceSymbol,
) (int, error) {
	if err := validateObjectiveEvidenceLine(
		"repository symbol ID", symbol.ID, maxObjectiveEvidenceSourceRefBytes-2,
	); err != nil {
		return 0, err
	}
	if !validObjectiveSHA256(symbol.SourceSHA256) {
		return 0, fmt.Errorf("repository evidence symbol %q requires an exact source SHA-256", symbol.ID)
	}
	if len(packID)+1+len(symbol.ID) > maxObjectiveEvidenceSourceRefBytes {
		return 0, fmt.Errorf("repository evidence provenance reference exceeds %d bytes", maxObjectiveEvidenceSourceRefBytes)
	}
	parts := []string{symbol.Kind, symbol.Name, symbol.Signature, symbol.Source}
	projectedBytes := len(parts) - 1
	hasContent := false
	for _, part := range parts {
		if !utf8.ValidString(part) || strings.ContainsRune(part, '\x00') {
			return 0, fmt.Errorf("repository evidence symbol %q contains invalid UTF-8 or NUL", symbol.ID)
		}
		hasContent = hasContent || strings.TrimSpace(part) != ""
		projectedBytes += len(part)
	}
	if projectedBytes > maxObjectiveRepositoryEvidenceTextBytes {
		return 0, fmt.Errorf(
			"repository evidence symbol %q exceeds the %d-byte capsule limit before projection",
			symbol.ID, maxObjectiveRepositoryEvidenceTextBytes,
		)
	}
	if !hasContent {
		return 0, fmt.Errorf("repository evidence symbol %q has no bounded public content", symbol.ID)
	}
	return projectedBytes, nil
}
