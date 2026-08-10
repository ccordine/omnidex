package retrieval

import (
	"encoding/json"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func addEvidenceSymbols(
	pack *EvidencePack,
	snapshot repositoryfacts.Snapshot,
	selected []scoredSymbol,
	limits Limits,
) error {
	for _, item := range selected {
		symbol := EvidenceSymbol{
			ID: item.symbol.ID, Kind: item.symbol.Kind, Name: item.symbol.Name,
			Signature: item.symbol.Signature, SourceSHA256: item.symbol.SourceSHA256,
			MatchScore: item.score,
		}
		var omission *SourceOmission
		if item.symbol.EndByte-item.symbol.StartByte > int64(limits.MaxSpanBytes) {
			omission = &SourceOmission{SymbolID: item.symbol.ID, Reason: "source_span_exceeds_limit"}
		} else {
			span, err := repositoryfacts.ReadExactSymbolSpan(snapshot, item.symbol, limits.MaxSpanBytes)
			if err != nil {
				return err
			}
			symbol.Source = span.Content
		}
		candidate := *pack
		candidate.Symbols = append(append([]EvidenceSymbol(nil), pack.Symbols...), symbol)
		if omission != nil {
			candidate.SourceOmissions = append(append([]SourceOmission(nil), pack.SourceOmissions...), *omission)
		}
		if !evidencePackFits(candidate) {
			return fmt.Errorf("repository evidence pack budget cannot hold required symbol %q", item.symbol.ID)
		}
		*pack = candidate
	}
	return nil
}

func addOmittedSymbol(pack *EvidencePack, symbolID string) error {
	for _, existing := range pack.OmittedSymbolIDs {
		if existing == symbolID {
			return nil
		}
	}
	candidate := *pack
	candidate.OmittedSymbolIDs = append(append([]string(nil), pack.OmittedSymbolIDs...), symbolID)
	if !evidencePackFits(candidate) {
		return fmt.Errorf("repository evidence pack budget cannot represent symbol omissions")
	}
	*pack = candidate
	return nil
}

func addEvidenceRelations(
	pack *EvidencePack,
	edges []repositoryfacts.Edge,
	included map[string]struct{},
	requiredTarget string,
) {
	for _, edge := range edges {
		_, fromIncluded := included[edge.FromID]
		_, toIncluded := included[edge.ToID]
		if !fromIncluded || !toIncluded || (requiredTarget != "" && edge.ToID != requiredTarget) {
			pack.OmittedEdges++
			continue
		}
		relation := EvidenceRelation{
			ID: edge.ID, FromID: edge.FromID, ToID: edge.ToID, Kind: edge.Kind,
			Origin: string(edge.Origin), Confidence: edge.Confidence,
		}
		candidate := *pack
		candidate.Relations = append(append([]EvidenceRelation(nil), pack.Relations...), relation)
		if evidencePackFits(candidate) {
			*pack = candidate
			continue
		}
		pack.OmittedEdges++
	}
}

func evidencePackFits(pack EvidencePack) bool {
	pack.ID = "evidence_pack_" + strings.Repeat("0", 64)
	raw, err := json.Marshal(pack)
	return err == nil && len(raw) <= pack.MaxBytes
}
