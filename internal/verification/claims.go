package verification

import (
	"sort"
	"strings"
	"unicode"

	"github.com/gryph/omnidex/internal/evidence"
)

type ClaimAssessment struct {
	Text         string
	Normalized   string
	Supported    bool
	SupportScore float64
	EvidenceRefs []int64
	Rationale    string
}

func AssessClaims(response string, records []evidence.Record, limit int) []ClaimAssessment {
	claims := ExtractClaims(response, limit)
	if len(claims) == 0 {
		return nil
	}
	prepared := make([]preparedEvidence, 0, len(records))
	for _, record := range records {
		prepared = append(prepared, prepareEvidence(record))
	}
	out := make([]ClaimAssessment, 0, len(claims))
	for _, claim := range claims {
		claimTokens := tokenSet(claim)
		if len(claimTokens) == 0 {
			continue
		}
		hits := make([]supportHit, 0, 4)
		for _, item := range prepared {
			score := overlapScore(claimTokens, item.tokens)
			if score <= 0 {
				continue
			}
			hits = append(hits, supportHit{id: item.id, score: score})
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		assessment := ClaimAssessment{Text: claim, Normalized: normalizeClaim(claim)}
		for idx, hit := range hits {
			if idx >= 3 {
				break
			}
			assessment.EvidenceRefs = append(assessment.EvidenceRefs, hit.id)
			assessment.SupportScore += hit.score
		}
		assessment.Supported = assessment.SupportScore >= 2.0
		if assessment.Supported {
			assessment.Rationale = "claim shares concrete terminology with captured evidence"
		} else {
			assessment.Rationale = "claim lacks enough overlap with captured evidence"
		}
		out = append(out, assessment)
	}
	return out
}

func ExtractClaims(response string, limit int) []string {
	response = strings.TrimSpace(response)
	if response == "" {
		return nil
	}
	segments := strings.FieldsFunc(strings.ReplaceAll(response, "\r\n", "\n"), func(r rune) bool {
		return r == '\n' || r == '.' || r == '!' || r == '?'
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(segments), limit))
	for _, segment := range segments {
		claim := strings.TrimSpace(segment)
		if len(claim) < 20 {
			continue
		}
		lower := strings.ToLower(claim)
		if strings.HasPrefix(lower, "source:") || strings.HasPrefix(lower, "sources:") || strings.HasPrefix(lower, "url:") {
			continue
		}
		normalized := normalizeClaim(claim)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, claim)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

type preparedEvidence struct {
	id     int64
	tokens map[string]struct{}
}

type supportHit struct {
	id    int64
	score float64
}

func prepareEvidence(record evidence.Record) preparedEvidence {
	parts := []string{record.Summary, record.Excerpt, record.SourceRef, record.Command, strings.Join(record.FilePaths, " ")}
	return preparedEvidence{id: record.ID, tokens: tokenSet(strings.Join(parts, " "))}
}

func overlapScore(claimTokens, evidenceTokens map[string]struct{}) float64 {
	if len(claimTokens) == 0 || len(evidenceTokens) == 0 {
		return 0
	}
	score := 0.0
	for token := range claimTokens {
		if _, ok := evidenceTokens[token]; ok {
			score += 1.0
		}
	}
	return score
}

func normalizeClaim(value string) string {
	return strings.Join(sortedTokens(value), " ")
}

func tokenSet(value string) map[string]struct{} {
	tokens := sortedTokens(value)
	out := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		out[token] = struct{}{}
	}
	return out
}

func sortedTokens(value string) []string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/')
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "-_/.")
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
