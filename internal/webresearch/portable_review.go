package webresearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (stations *PortableStations) Review(
	ctx context.Context,
	call ClaimEvidenceReviewCall,
) (ClaimEvidenceReviewDecision, error) {
	if err := validatePortableClaimEvidenceReviewCall(call); err != nil {
		return ClaimEvidenceReviewDecision{}, err
	}
	evidence := make([]assemblyline.WebReviewEvidence, len(call.Evidence))
	evidenceIDs := make([]string, len(call.Evidence))
	for index, item := range call.Evidence {
		evidence[index] = assemblyline.WebReviewEvidence{
			EvidenceID: string(item.EvidenceID), Title: item.Title,
			Snippet: item.Snippet, Content: item.Content,
		}
		evidenceIDs[index] = string(item.EvidenceID)
	}
	input := assemblyline.WebClaimEvidenceReviewInput{
		ExactQuestion: call.Question,
		Context:       assemblyline.CloneObjectiveContext(call.Context),
		Paragraph: assemblyline.WebReviewParagraph{
			ParagraphID: string(call.ParagraphID), Text: call.ParagraphText, EvidenceIDs: evidenceIDs,
		},
		Evidence: evidence,
	}
	acceptedClaims := make([]string, 0, assemblyline.MaxWebReviewClaims)
	semanticCalls := 0
	for {
		claimInput := assemblyline.WebReviewClaimLeafInput{
			ExactQuestion:  input.ExactQuestion,
			Context:        assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText:  input.Paragraph.Text,
			AcceptedClaims: append([]string{}, acceptedClaims...),
		}
		if len(acceptedClaims) > 0 {
			coverageJob, err := assemblyline.NewWebReviewClaimCoverageJob(claimInput)
			if err != nil {
				return ClaimEvidenceReviewDecision{}, fmt.Errorf("build web review claim coverage job: %w", err)
			}
			coverage, err := runPortableSemanticLeaf(
				ctx, stations, coverageJob,
				func(raw string) (assemblyline.WebReviewClaimCoverageDecision, error) {
					return assemblyline.DecodeWebReviewClaimCoverageDecision(claimInput, raw)
				},
			)
			semanticCalls++
			if err != nil {
				return ClaimEvidenceReviewDecision{}, err
			}
			if coverage.Coverage == assemblyline.WebReviewNoUncoveredClaim {
				decision := assemblyline.WebClaimEvidenceReviewDecision{
					Schema:  assemblyline.WebClaimEvidenceReviewSchemaV1,
					Outcome: assemblyline.WebClaimEvidenceReviewNone,
				}
				if err := decision.ValidateFor(input); err != nil {
					return ClaimEvidenceReviewDecision{}, err
				}
				return portableClaimEvidenceReviewDecision(decision, semanticCalls), nil
			}
		}
		if len(acceptedClaims) >= assemblyline.MaxWebReviewClaims {
			return ClaimEvidenceReviewDecision{}, fmt.Errorf(
				"web review still requires another claim after the %d-claim bound",
				assemblyline.MaxWebReviewClaims,
			)
		}
		claimJob, err := assemblyline.NewWebReviewClaimJob(claimInput)
		if err != nil {
			return ClaimEvidenceReviewDecision{}, fmt.Errorf("build web review claim job: %w", err)
		}
		claim, err := runPortableSemanticLeaf(
			ctx, stations, claimJob,
			func(raw string) (assemblyline.WebReviewClaimDecision, error) {
				return assemblyline.DecodeWebReviewClaimDecision(claimInput, raw)
			},
		)
		semanticCalls++
		if err != nil {
			return ClaimEvidenceReviewDecision{}, err
		}
		acceptedClaims = append(acceptedClaims, claim.Claim)

		verdictInput := assemblyline.WebReviewClaimVerdictInput{
			ExactQuestion: input.ExactQuestion,
			Context:       assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText: input.Paragraph.Text, Claim: claim.Claim,
			Evidence: append([]assemblyline.WebReviewEvidence(nil), input.Evidence...),
		}
		verdictJob, err := assemblyline.NewWebReviewClaimVerdictJob(verdictInput)
		if err != nil {
			return ClaimEvidenceReviewDecision{}, fmt.Errorf("build web review claim verdict job: %w", err)
		}
		verdict, err := runPortableSemanticLeaf(
			ctx, stations, verdictJob,
			func(raw string) (assemblyline.WebReviewClaimVerdictDecision, error) {
				return assemblyline.DecodeWebReviewClaimVerdictDecision(verdictInput, raw)
			},
		)
		semanticCalls++
		if err != nil {
			return ClaimEvidenceReviewDecision{}, err
		}
		issueKind, issue := verdict.IssueKind()
		if !issue {
			continue
		}
		decision, issueCalls, err := stations.resolveWebReviewIssue(
			ctx, input, claim.Claim, issueKind,
		)
		semanticCalls += issueCalls
		if err != nil {
			return ClaimEvidenceReviewDecision{}, err
		}
		return portableClaimEvidenceReviewDecision(decision, semanticCalls), nil
	}
}

func (stations *PortableStations) resolveWebReviewIssue(
	ctx context.Context,
	input assemblyline.WebClaimEvidenceReviewInput,
	claim string,
	issueKind assemblyline.WebClaimEvidenceIssueKind,
) (assemblyline.WebClaimEvidenceReviewDecision, int, error) {
	ids := make([]string, 0, len(input.Evidence))
	implicated := make([]assemblyline.WebReviewEvidence, 0, len(input.Evidence))
	calls := 0
	for _, evidence := range input.Evidence {
		relationInput := assemblyline.WebReviewIssueEvidenceRelationInput{
			ExactQuestion: input.ExactQuestion,
			Context:       assemblyline.CloneObjectiveContext(input.Context),
			ParagraphText: input.Paragraph.Text, Claim: claim,
			IssueKind: issueKind, Evidence: evidence,
		}
		job, err := assemblyline.NewWebReviewIssueEvidenceRelationJob(relationInput)
		if err != nil {
			return assemblyline.WebClaimEvidenceReviewDecision{}, calls, fmt.Errorf("build web review issue evidence relation job: %w", err)
		}
		relation, err := runPortableSemanticLeaf(
			ctx, stations, job,
			func(raw string) (assemblyline.WebReviewIssueEvidenceRelationDecision, error) {
				return assemblyline.DecodeWebReviewIssueEvidenceRelationDecision(relationInput, raw)
			},
		)
		calls++
		if err != nil {
			return assemblyline.WebClaimEvidenceReviewDecision{}, calls, err
		}
		if relation.Relation == assemblyline.WebReviewEvidenceImplicated {
			ids = append(ids, evidence.EvidenceID)
			implicated = append(implicated, evidence)
		}
	}
	if len(ids) == 0 {
		return assemblyline.WebClaimEvidenceReviewDecision{}, calls,
			fmt.Errorf("web review issue has no semantically implicated evidence")
	}
	detailInput := assemblyline.WebReviewIssueDetailInput{
		ExactQuestion: input.ExactQuestion,
		Context:       assemblyline.CloneObjectiveContext(input.Context),
		ParagraphText: input.Paragraph.Text, Claim: claim,
		IssueKind: issueKind, Evidence: implicated,
	}
	detailJob, err := assemblyline.NewWebReviewIssueDetailJob(detailInput)
	if err != nil {
		return assemblyline.WebClaimEvidenceReviewDecision{}, calls, fmt.Errorf("build web review issue detail job: %w", err)
	}
	detail, err := runPortableSemanticLeaf(
		ctx, stations, detailJob,
		func(raw string) (assemblyline.WebReviewIssueDetailDecision, error) {
			return assemblyline.DecodeWebReviewIssueDetailDecision(detailInput, raw)
		},
	)
	calls++
	if err != nil {
		return assemblyline.WebClaimEvidenceReviewDecision{}, calls, err
	}
	decision := assemblyline.WebClaimEvidenceReviewDecision{
		Schema:      assemblyline.WebClaimEvidenceReviewSchemaV1,
		Outcome:     assemblyline.WebClaimEvidenceReviewIssue,
		ParagraphID: input.Paragraph.ParagraphID,
		EvidenceIDs: ids, IssueKind: issueKind, Detail: detail.Detail,
	}
	if err := decision.ValidateFor(input); err != nil {
		return assemblyline.WebClaimEvidenceReviewDecision{}, calls, err
	}
	return decision, calls, nil
}

func portableClaimEvidenceReviewDecision(
	decision assemblyline.WebClaimEvidenceReviewDecision,
	semanticCalls int,
) ClaimEvidenceReviewDecision {
	ids := make([]EvidenceID, len(decision.EvidenceIDs))
	for index, id := range decision.EvidenceIDs {
		ids[index] = EvidenceID(id)
	}
	return ClaimEvidenceReviewDecision{
		Outcome:     ClaimEvidenceReviewOutcome(decision.Outcome),
		ParagraphID: ParagraphID(decision.ParagraphID), EvidenceIDs: ids,
		IssueKind: ClaimEvidenceIssueKind(decision.IssueKind), Detail: decision.Detail,
		SemanticCalls: semanticCalls,
	}
}

func validatePortableClaimEvidenceReviewCall(call ClaimEvidenceReviewCall) error {
	if err := validatePortableQuestion(call.Question); err != nil {
		return err
	}
	if err := validatePortableObjectiveContext(call.Question, call.Context); err != nil {
		return err
	}
	if err := validatePortableIdentity(string(call.ParagraphID)); err != nil {
		return err
	}
	if err := validatePortableField(call.ParagraphText, maxPortableSynthesisParagraphBytes); err != nil ||
		strings.TrimSpace(call.ParagraphText) == "" {
		return fmt.Errorf("portable claim-evidence review paragraph is invalid")
	}
	if len(call.Evidence) < 1 || len(call.Evidence) > maxPortableReviewEvidence {
		return fmt.Errorf("portable claim-evidence review requires 1..%d cited evidence capsules", maxPortableReviewEvidence)
	}
	total := 0
	seen := make(map[EvidenceID]struct{}, len(call.Evidence))
	for _, item := range call.Evidence {
		if err := validatePortableIdentity(string(item.EvidenceID)); err != nil {
			return err
		}
		if _, duplicate := seen[item.EvidenceID]; duplicate {
			return fmt.Errorf("portable claim-evidence review evidence ID %q is duplicated", item.EvidenceID)
		}
		seen[item.EvidenceID] = struct{}{}
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("portable claim-evidence review evidence %q has no content", item.EvidenceID)
		}
		for _, value := range []string{item.Title, item.Snippet, item.Content} {
			if err := validatePortableField(value, maxPortableEvidenceFieldBytes); err != nil {
				return err
			}
		}
		total += len(item.EvidenceID) + len(item.Title) + len(item.Snippet) + len(item.Content)
	}
	if total > maxPortableEvidenceProjection {
		return fmt.Errorf("portable claim-evidence review projection exceeds %d bytes", maxPortableEvidenceProjection)
	}
	return nil
}
