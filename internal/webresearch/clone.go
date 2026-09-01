package webresearch

import (
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func cloneObjective(value Objective) Objective {
	value.KnownArtifactPaths = append([]string{}, value.KnownArtifactPaths...)
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
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

func cloneRelevanceCall(value RelevanceCall) RelevanceCall {
	value.Context = assemblyline.CloneObjectiveContext(value.Context)
	value.Candidates = append([]RelevanceCandidate{}, value.Candidates...)
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
