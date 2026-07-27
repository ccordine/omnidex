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
		if record.Kind == evidence.KindMemoryExcerpt || record.Kind == evidence.KindModelJudgment {
			continue
		}
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
			score, coverage := overlapMetrics(claimTokens, item.tokens)
			if score <= 0 {
				continue
			}
			hits = append(hits, supportHit{id: item.id, score: score, coverage: coverage})
		}
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].coverage == hits[j].coverage {
				return hits[i].score > hits[j].score
			}
			return hits[i].coverage > hits[j].coverage
		})
		assessment := ClaimAssessment{Text: claim, Normalized: normalizeClaim(claim)}
		for idx, hit := range hits {
			if idx >= 3 || hit.score < 2 || hit.coverage < 0.35 {
				break
			}
			assessment.EvidenceRefs = append(assessment.EvidenceRefs, hit.id)
		}
		if len(hits) > 0 {
			assessment.SupportScore = hits[0].coverage
			assessment.Supported = hits[0].score >= 2 && hits[0].coverage >= 0.35
		}
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
	id       int64
	score    float64
	coverage float64
}

func prepareEvidence(record evidence.Record) preparedEvidence {
	parts := []string{record.Summary, record.Excerpt, record.SourceRef, record.Command, strings.Join(record.FilePaths, " ")}
	return preparedEvidence{id: record.ID, tokens: tokenSet(strings.Join(parts, " "))}
}

func overlapMetrics(claimTokens, evidenceTokens map[string]struct{}) (float64, float64) {
	if len(claimTokens) == 0 || len(evidenceTokens) == 0 {
		return 0, 0
	}
	score := 0.0
	for token := range claimTokens {
		if _, ok := evidenceTokens[token]; ok {
			score += 1.0
		}
	}
	return score, score / float64(len(claimTokens))
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
		if cjkTokens := cjkNGrams(field); len(cjkTokens) > 0 {
			for _, token := range cjkTokens {
				if _, ok := seen[token]; ok {
					continue
				}
				seen[token] = struct{}{}
				out = append(out, token)
			}
			continue
		}
		if len([]rune(field)) < 3 || verificationStopword(field) {
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

func cjkNGrams(value string) []string {
	runes := []rune(value)
	containsCJK := false
	for _, current := range runes {
		if isCJKRune(current) {
			containsCJK = true
			break
		}
	}
	if !containsCJK {
		return nil
	}
	out := make([]string, 0, len(runes)*2)
	for size := 2; size <= 3; size++ {
		for start := 0; start+size <= len(runes); start++ {
			valid := true
			for _, current := range runes[start : start+size] {
				if !isCJKRune(current) {
					valid = false
					break
				}
			}
			if valid {
				out = append(out, string(runes[start:start+size]))
			}
		}
	}
	return out
}

func isCJKRune(value rune) bool {
	return unicode.In(value, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func verificationStopword(value string) bool {
	switch value {
	case "and", "are", "but", "for", "from", "has", "have", "into", "not", "that", "the", "their", "this", "was", "were", "with",
		"con", "del", "las", "los", "para", "por", "que", "una", "uno",
		"для", "или", "как", "при", "что", "это":
		return true
	default:
		return false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
