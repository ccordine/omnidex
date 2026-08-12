package repositoryobjective

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const (
	maxRepositoryFiles     = 1024
	maxSubjectCandidates   = 8
	maxDeclarationBytes    = 64 * 1024
	maxGapDeclarationBytes = 2 * 1024
)

type subjectCandidate struct {
	symbol   repositoryfacts.Symbol
	span     repositoryfacts.SourceSpan
	evidence SymbolEvidence
}

func discoverCandidates(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	lookup SubjectLookup,
) ([]subjectCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	matches := make([]repositoryfacts.Symbol, 0, 2)
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	for _, symbol := range analysis.Symbols {
		if symbol.Kind != "function" && symbol.Kind != "method" {
			continue
		}
		file, exists := files[symbol.FileID]
		if !exists {
			return nil, fmt.Errorf("%w: subject %q references an unknown file", ErrRepositoryAuthority, symbol.ID)
		}
		if file.Test {
			continue
		}
		matched := lookup.Kind == LookupQualifiedName && symbol.QualifiedName == lookup.Value
		matched = matched || lookup.Kind == LookupName && symbol.Name == lookup.Value
		if matched {
			matches = append(matches, symbol)
		}
	}
	sort.Slice(matches, func(left, right int) bool { return matches[left].ID < matches[right].ID })
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrSubjectAbsent, lookup.Value)
	}
	if lookup.Kind == LookupQualifiedName && len(matches) != 1 {
		return nil, fmt.Errorf("%w: qualified name %q has %d exact declarations", ErrRepositoryAuthority, lookup.Value, len(matches))
	}
	if len(matches) > maxSubjectCandidates {
		return nil, fmt.Errorf("%w: name %q has %d candidates; maximum is %d", ErrSubjectAmbiguous, lookup.Value, len(matches), maxSubjectCandidates)
	}
	candidates := make([]subjectCandidate, 0, len(matches))
	for _, symbol := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		span, err := repositoryfacts.ReadExactSymbolSpan(snapshot, symbol, maxDeclarationBytes)
		if err != nil {
			return nil, fmt.Errorf("inspect subject candidate %q: %w", symbol.ID, err)
		}
		candidates = append(candidates, subjectCandidate{
			symbol: symbol, span: span, evidence: symbolEvidence(symbol, span),
		})
	}
	return candidates, nil
}

func symbolEvidence(symbol repositoryfacts.Symbol, span repositoryfacts.SourceSpan) SymbolEvidence {
	digest := sha256.Sum256([]byte(span.Content))
	return SymbolEvidence{
		SymbolID: symbol.ID, QualifiedName: symbol.QualifiedName, Kind: symbol.Kind,
		Signature: symbol.Signature, SourceSHA256: symbol.SourceSHA256,
		DeclarationSHA256: hex.EncodeToString(digest[:]),
	}
}
