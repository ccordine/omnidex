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
	accepted := make([]assemblyline.WebGroundedParagraph, 0, call.MaxParagraphs)
	semanticCalls := 0
	for {
		leafInput := webSynthesisParagraphLeafInput(base, accepted)
		if len(accepted) > 0 {
			coverageJob, err := assemblyline.NewWebSynthesisParagraphCoverageJob(leafInput)
			if err != nil {
				return GroundedSynthesisDecision{}, fmt.Errorf("build web synthesis coverage job: %w", err)
			}
			coverage, err := runPortableSemanticLeaf(
				ctx, stations, coverageJob,
				func(raw string) (assemblyline.WebSynthesisParagraphCoverageDecision, error) {
					return assemblyline.DecodeWebSynthesisParagraphCoverageDecision(leafInput, raw)
				},
			)
			semanticCalls++
			if err != nil {
				return GroundedSynthesisDecision{}, err
			}
			if coverage.Coverage == assemblyline.WebSynthesisNoUncoveredParagraph {
				assembled := assemblyline.WebGroundedSynthesisDecision{
					Schema:     assemblyline.WebGroundedSynthesisSchemaV1,
					Paragraphs: append([]assemblyline.WebGroundedParagraph(nil), accepted...),
				}
				if err := assembled.ValidateFor(base); err != nil {
					return GroundedSynthesisDecision{}, err
				}
				return portableGroundedSynthesisDecision(assembled, semanticCalls), nil
			}
		}
		if len(accepted) >= call.MaxParagraphs {
			return GroundedSynthesisDecision{}, fmt.Errorf(
				"web synthesis still requires another paragraph after the %d-paragraph bound",
				call.MaxParagraphs,
			)
		}

		paragraphJob, err := assemblyline.NewWebSynthesisParagraphJob(leafInput)
		if err != nil {
			return GroundedSynthesisDecision{}, fmt.Errorf("build web synthesis paragraph job: %w", err)
		}
		paragraph, err := runPortableSemanticLeaf(
			ctx, stations, paragraphJob,
			func(raw string) (assemblyline.WebSynthesisParagraphDecision, error) {
				return assemblyline.DecodeWebSynthesisParagraphDecision(leafInput, raw)
			},
		)
		semanticCalls++
		if err != nil {
			return GroundedSynthesisDecision{}, err
		}
		boundIDs, relationCalls, err := stations.bindWebSynthesisEvidence(
			ctx, base, paragraph.Text,
		)
		semanticCalls += relationCalls
		if err != nil {
			return GroundedSynthesisDecision{}, err
		}
		accepted = append(accepted, assemblyline.WebGroundedParagraph{
			Text: paragraph.Text, EvidenceIDs: boundIDs,
		})
	}
}

func webSynthesisParagraphLeafInput(
	base assemblyline.WebGroundedSynthesisInput,
	accepted []assemblyline.WebGroundedParagraph,
) assemblyline.WebSynthesisParagraphLeafInput {
	cloned := make([]assemblyline.WebGroundedParagraph, len(accepted))
	for index, paragraph := range accepted {
		cloned[index] = paragraph
		cloned[index].EvidenceIDs = append([]string(nil), paragraph.EvidenceIDs...)
	}
	return assemblyline.WebSynthesisParagraphLeafInput{
		ExactQuestion:      base.ExactQuestion,
		Context:            assemblyline.CloneObjectiveContext(base.Context),
		Evidence:           append([]assemblyline.WebGroundedEvidence(nil), base.Evidence...),
		AcceptedParagraphs: cloned,
		MaxParagraphs:      base.MaxParagraphs, MaxParagraphBytes: base.MaxParagraphBytes,
	}
}

func (stations *PortableStations) bindWebSynthesisEvidence(
	ctx context.Context,
	base assemblyline.WebGroundedSynthesisInput,
	paragraphText string,
) ([]string, int, error) {
	limit := min(len(base.Evidence), maxPortableReviewEvidence)
	bound := make([]string, 0, limit)
	calls := 0
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
			return nil, calls, fmt.Errorf("build web synthesis evidence relation job: %w", err)
		}
		relation, err := runPortableSemanticLeaf(
			ctx, stations, job,
			func(raw string) (assemblyline.WebSynthesisEvidenceRelationDecision, error) {
				return assemblyline.DecodeWebSynthesisEvidenceRelationDecision(input, raw)
			},
		)
		calls++
		if err != nil {
			return nil, calls, err
		}
		if relation.Relation == assemblyline.WebEvidenceSupportsParagraph {
			bound = append(bound, evidence.EvidenceID)
		}
	}
	if len(bound) == 0 {
		return nil, calls, fmt.Errorf("web synthesis paragraph has no semantically bound evidence")
	}
	return bound, calls, nil
}

func portableGroundedSynthesisDecision(
	decision assemblyline.WebGroundedSynthesisDecision,
	semanticCalls int,
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
		Paragraphs: paragraphs, SemanticCalls: semanticCalls,
	}
}
