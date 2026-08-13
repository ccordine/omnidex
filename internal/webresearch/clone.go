package webresearch

import (
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func cloneObjective(value Objective) Objective {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.Acceptance = append([]AcceptancePredicate{}, value.Acceptance...)
	return value
}

func cloneCandidates(values []websearch.Candidate) []websearch.Candidate {
	result := make([]websearch.Candidate, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Sources = append([]websearch.CandidateSource{}, value.Sources...)
	}
	return result
}

func cloneCandidateReport(value websearch.CandidateReport) websearch.CandidateReport {
	value.Candidates = cloneCandidates(value.Candidates)
	value.Diagnostics = append([]websearch.ProviderDiagnostic{}, value.Diagnostics...)
	return value
}

func cloneDocumentReport(value websearch.DocumentReport) websearch.DocumentReport {
	value.Documents = append([]websearch.Document{}, value.Documents...)
	value.Diagnostics = append([]websearch.DocumentDiagnostic{}, value.Diagnostics...)
	return value
}

func cloneSearchTermsCall(value SearchTermsCall) SearchTermsCall {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.AttemptedQueries = append([]string{}, value.AttemptedQueries...)
	return value
}

func cloneRelevanceCall(value RelevanceCall) RelevanceCall {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.Candidates = append([]RelevanceCandidate{}, value.Candidates...)
	return value
}

func cloneClaimEvidenceReviewCall(value ClaimEvidenceReviewCall) ClaimEvidenceReviewCall {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.Evidence = cloneProjection(value.Evidence)
	return value
}

func cloneClaimEvidenceReviewDecision(value ClaimEvidenceReviewDecision) ClaimEvidenceReviewDecision {
	value.EvidenceIDs = append([]EvidenceID{}, value.EvidenceIDs...)
	return value
}

func cloneGroundedSynthesisCorrectionCall(value GroundedSynthesisCorrectionCall) GroundedSynthesisCorrectionCall {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.Paragraphs = cloneParagraphs(value.Paragraphs)
	value.Issue = cloneClaimEvidenceReviewDecision(value.Issue)
	value.Evidence = cloneProjection(value.Evidence)
	return value
}

func cloneEvidence(values []Evidence) []Evidence { return append([]Evidence{}, values...) }

func cloneProjection(values []ProjectedEvidence) []ProjectedEvidence {
	return append([]ProjectedEvidence{}, values...)
}

func cloneParagraphs(values []GroundedParagraph) []GroundedParagraph {
	result := make([]GroundedParagraph, len(values))
	for index, value := range values {
		result[index] = value
		result[index].EvidenceIDs = append([]EvidenceID{}, value.EvidenceIDs...)
	}
	return result
}

func cloneArtifact(value Artifact) Artifact {
	value.Paragraphs = cloneParagraphs(value.Paragraphs)
	value.Sources = append([]CitationSource{}, value.Sources...)
	return value
}

func cloneResult(value Result) Result {
	value.Objective = cloneObjective(value.Objective)
	value.Steps = append([]Step{}, value.Steps...)
	discovery := value.Discovery
	value.Discovery = make([]websearch.CandidateReport, len(discovery))
	for index, report := range discovery {
		value.Discovery[index] = cloneCandidateReport(report)
	}
	fetches := value.Fetches
	value.Fetches = make([]websearch.DocumentReport, len(fetches))
	for index, report := range fetches {
		value.Fetches[index] = cloneDocumentReport(report)
	}
	value.Evidence = cloneEvidence(value.Evidence)
	value.Projected = cloneProjection(value.Projected)
	value.Artifact = cloneArtifact(value.Artifact)
	return value
}

func truncateBytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
