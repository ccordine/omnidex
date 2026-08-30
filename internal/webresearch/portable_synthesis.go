package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (stations *PortableStations) Synthesize(
	ctx context.Context,
	call GroundedSynthesisCall,
) (GroundedSynthesisDecision, error) {
	if err := validatePortableSynthesisCall(call); err != nil {
		return GroundedSynthesisDecision{}, err
	}
	evidence := make([]assemblyline.WebGroundedEvidence, len(call.Evidence))
	for index, item := range call.Evidence {
		evidence[index] = assemblyline.WebGroundedEvidence{
			EvidenceID: string(item.EvidenceID), Title: item.Title,
			Snippet: item.Snippet, Content: item.Content,
		}
	}
	base := assemblyline.WebGroundedSynthesisInput{
		ExactQuestion: call.Question,
		Context:       assemblyline.CloneObjectiveContext(call.Context),
		Evidence:      evidence,
		MaxParagraphs: call.MaxParagraphs, MaxParagraphBytes: call.MaxParagraphBytes,
	}
	inventoryJob, err := assemblyline.NewWebSynthesisParagraphInventoryJob(base)
	if err != nil {
		return GroundedSynthesisDecision{}, fmt.Errorf("build web synthesis paragraph inventory job: %w", err)
	}
	inventory, inventoryReceipt, err := runPortableSemanticLeaf(
		ctx, stations, inventoryJob,
		func(raw string) (assemblyline.WebSynthesisParagraphInventory, error) {
			return assemblyline.DecodeWebSynthesisParagraphInventory(base, raw)
		},
	)
	if err != nil {
		return GroundedSynthesisDecision{}, err
	}
	var ledger SemanticCallLedger
	if err := ledger.Record(
		"web synthesis paragraph inventory", inventoryReceipt,
		exactPortableSemanticLeafCalls,
	); err != nil {
		return GroundedSynthesisDecision{}, err
	}
	accepted := make([]assemblyline.WebGroundedParagraph, 0, call.MaxParagraphs)
	seenCandidates := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := seenCandidates[candidate]; duplicate {
			continue
		}
		seenCandidates[candidate] = struct{}{}
		authorizationInput := assemblyline.WebSynthesisParagraphAuthorizationInput{
			ExactQuestion:     base.ExactQuestion,
			Context:           assemblyline.CloneObjectiveContext(base.Context),
			ParagraphText:     candidate,
			Evidence:          append([]assemblyline.WebGroundedEvidence(nil), base.Evidence...),
			MaxParagraphBytes: base.MaxParagraphBytes,
		}
		authorizationJob, err := assemblyline.NewWebSynthesisParagraphAuthorizationJob(
			authorizationInput,
		)
		if err != nil {
			return GroundedSynthesisDecision{}, fmt.Errorf(
				"build web synthesis paragraph authorization job: %w", err,
			)
		}
		authorization, authorizationReceipt, err := runPortableSemanticLeaf(
			ctx, stations, authorizationJob,
			func(raw string) (assemblyline.WebSynthesisParagraphAuthorizationDecision, error) {
				return assemblyline.DecodeWebSynthesisParagraphAuthorizationDecision(
					authorizationInput, raw,
				)
			},
		)
		if err != nil {
			return GroundedSynthesisDecision{}, err
		}
		if err := ledger.Record(
			"web synthesis paragraph authorization",
			authorizationReceipt,
			exactPortableSemanticLeafCalls,
		); err != nil {
			return GroundedSynthesisDecision{}, err
		}
		if authorization.Relation != assemblyline.WebParagraphResponsiveAndFullySupported {
			continue
		}
		boundIDs, relationLedger, err := stations.bindWebSynthesisEvidence(
			ctx, base, candidate,
		)
		if err != nil {
			return GroundedSynthesisDecision{}, err
		}
		if err := ledger.Merge("web synthesis evidence", relationLedger); err != nil {
			return GroundedSynthesisDecision{}, err
		}
		if len(boundIDs) == 0 {
			continue
		}
		accepted = append(accepted, assemblyline.WebGroundedParagraph{
			Text: candidate, EvidenceIDs: boundIDs,
		})
	}
	if len(accepted) == 0 {
		return GroundedSynthesisDecision{}, fmt.Errorf(
			"web synthesis paragraph inventory queue produced no responsive fully supported paragraphs",
		)
	}
	assembled := assemblyline.WebGroundedSynthesisDecision{
		Schema:     assemblyline.WebGroundedSynthesisSchemaV1,
		Paragraphs: append([]assemblyline.WebGroundedParagraph(nil), accepted...),
	}
	if err := assembled.ValidateFor(base); err != nil {
		return GroundedSynthesisDecision{}, err
	}
	maximumCalls := (1 + call.MaxParagraphs*(len(call.Evidence)+1)) *
		exactPortableSemanticLeafCalls
	receipt, err := ledger.ValidateForMaximum("web grounded synthesis station", maximumCalls)
	if err != nil {
		return GroundedSynthesisDecision{}, err
	}
	return portableGroundedSynthesisDecision(assembled, receipt, ledger), nil
}

func (stations *PortableStations) bindWebSynthesisEvidence(
	ctx context.Context,
	base assemblyline.WebGroundedSynthesisInput,
	paragraphText string,
) ([]string, SemanticCallLedger, error) {
	limit := min(len(base.Evidence), maxEvidenceIDsPerParagraph)
	bound := make([]string, 0, limit)
	var ledger SemanticCallLedger
	for _, evidence := range base.Evidence {
		if len(bound) == limit {
			break
		}
		input := assemblyline.WebSynthesisEvidenceRelationInput{
			ExactQuestion: base.ExactQuestion,
			Context:       assemblyline.CloneObjectiveContext(base.Context),
			ParagraphText: paragraphText, Evidence: evidence,
		}
		job, err := assemblyline.NewWebSynthesisEvidenceRelationJob(input)
		if err != nil {
			return nil, ledger, fmt.Errorf("build web synthesis evidence relation job: %w", err)
		}
		relation, receipt, err := runPortableSemanticLeaf(
			ctx, stations, job,
			func(raw string) (assemblyline.WebSynthesisEvidenceRelationDecision, error) {
				return assemblyline.DecodeWebSynthesisEvidenceRelationDecision(input, raw)
			},
		)
		if err != nil {
			return nil, ledger, err
		}
		if err := ledger.Record(
			"web synthesis evidence "+evidence.EvidenceID,
			receipt,
			exactPortableSemanticLeafCalls,
		); err != nil {
			return nil, ledger, err
		}
		if relation.Relation == assemblyline.WebEvidenceSupportsParagraph {
			bound = append(bound, evidence.EvidenceID)
		}
	}
	return bound, ledger, nil
}

func portableGroundedSynthesisDecision(
	decision assemblyline.WebGroundedSynthesisDecision,
	receipt SemanticCallReceipt,
	ledger SemanticCallLedger,
) GroundedSynthesisDecision {
	paragraphs := make([]GroundedParagraph, len(decision.Paragraphs))
	for index, paragraph := range decision.Paragraphs {
		ids := make([]EvidenceID, len(paragraph.EvidenceIDs))
		for idIndex, id := range paragraph.EvidenceIDs {
			ids[idIndex] = EvidenceID(id)
		}
		paragraphs[index] = GroundedParagraph{Text: paragraph.Text, EvidenceIDs: ids}
	}
	return GroundedSynthesisDecision{
		Paragraphs: paragraphs, SemanticCalls: receipt.Calls,
		CallLedger: ledger.Clone(),
	}
}
