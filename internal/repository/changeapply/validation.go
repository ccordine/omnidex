package changeapply

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func resolveReplacements(input Input) ([]targetReplacement, error) {
	if input.Analysis.Adapter.Name != golangadapter.AdapterName {
		return nil, fmt.Errorf(
			"repository change staging has no final declaration validator for adapter %q",
			input.Analysis.Adapter.Name,
		)
	}
	files := make(map[string]repositoryfacts.File, len(input.Snapshot.Files))
	for _, file := range input.Snapshot.Files {
		files[file.ID] = file
	}
	symbols := make(map[string]repositoryfacts.Symbol, len(input.Analysis.Symbols))
	for _, symbol := range input.Analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	candidates := make(map[string]CandidateDeclaration, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if strings.TrimSpace(candidate.SymbolID) == "" {
			return nil, fmt.Errorf("repository change candidate requires one symbol ID")
		}
		if _, duplicate := candidates[candidate.SymbolID]; duplicate {
			return nil, fmt.Errorf("repository change candidate for symbol %q is duplicated", candidate.SymbolID)
		}
		if err := validateCandidate(candidate); err != nil {
			return nil, err
		}
		candidates[candidate.SymbolID] = candidate
	}
	replacements := make([]targetReplacement, 0, len(input.Contract.Targets))
	for _, target := range input.Contract.Targets {
		candidate, exists := candidates[target.SymbolID]
		if !exists {
			return nil, fmt.Errorf("repository change candidate for target %q is missing", target.SymbolID)
		}
		delete(candidates, target.SymbolID)
		file, exists := files[target.FileID]
		if !exists {
			return nil, fmt.Errorf("repository change target %q references an unknown file", target.SymbolID)
		}
		if err := validateMutableTarget(file, target); err != nil {
			return nil, err
		}
		symbol, exists := symbols[target.SymbolID]
		if !exists {
			return nil, fmt.Errorf("repository change target %q disappeared from exact analysis", target.SymbolID)
		}
		current, err := repositoryfacts.ReadExactSymbolSpan(
			input.Snapshot, symbol, maxCandidateDeclarationBytes,
		)
		if err != nil {
			return nil, err
		}
		permitted := make([]string, len(target.DirectCapabilities))
		for index, capability := range target.DirectCapabilities {
			permitted[index] = capability.Name
		}
		fragmentContract := gofragment.Contract{
			Signature: target.Signature, Current: current.Content,
			PermittedSymbols: permitted,
		}
		declaration, err := gofragment.ParseFunction(fragmentContract, candidate.Declaration)
		if err != nil {
			return nil, fmt.Errorf(
				"validate repository change candidate for symbol %q: %w",
				target.SymbolID, err,
			)
		}
		canonicalCurrent, err := gofragment.ParseFunction(fragmentContract, current.Content)
		if err != nil {
			return nil, fmt.Errorf(
				"validate current repository declaration for symbol %q: %w",
				target.SymbolID, err,
			)
		}
		if declaration == canonicalCurrent {
			return nil, fmt.Errorf(
				"repository change candidate for symbol %q is unchanged",
				target.SymbolID,
			)
		}
		replacements = append(replacements, targetReplacement{
			symbolID: target.SymbolID, fileID: target.FileID,
			start: target.StartByte, end: target.EndByte,
			expected:    target.ExpectedDeclarationSHA256,
			declaration: []byte(candidate.Declaration),
		})
	}
	if len(candidates) > 0 {
		ids := make([]string, 0, len(candidates))
		for id := range candidates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("repository change candidates contain extra symbol %q", ids[0])
	}
	if err := rejectOverlappingTargets(replacements); err != nil {
		return nil, err
	}
	return replacements, nil
}

func validateCandidate(candidate CandidateDeclaration) error {
	raw := []byte(candidate.Declaration)
	if len(raw) == 0 || strings.TrimSpace(candidate.Declaration) == "" {
		return fmt.Errorf("repository change candidate for symbol %q must be non-empty", candidate.SymbolID)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("repository change candidate for symbol %q must be valid UTF-8", candidate.SymbolID)
	}
	if strings.ContainsRune(candidate.Declaration, '\x00') {
		return fmt.Errorf("repository change candidate for symbol %q must be NUL-free", candidate.SymbolID)
	}
	if len(raw) > maxCandidateDeclarationBytes {
		return fmt.Errorf(
			"repository change candidate for symbol %q exceeds %d bytes",
			candidate.SymbolID, maxCandidateDeclarationBytes,
		)
	}
	if candidate.Declaration != strings.TrimSpace(candidate.Declaration) {
		return fmt.Errorf("repository change candidate for symbol %q must be trimmed", candidate.SymbolID)
	}
	if strings.ContainsRune(candidate.Declaration, '\r') {
		return fmt.Errorf("repository change candidate for symbol %q contains unsupported carriage-return bytes", candidate.SymbolID)
	}
	return nil
}

func validateMutableTarget(file repositoryfacts.File, target repositoryfacts.ChangeTarget) error {
	if file.Kind != repositoryfacts.EntryRegular {
		return fmt.Errorf("repository change target %q is an unsupported symlink or non-regular target", target.SymbolID)
	}
	if file.Generated || protectedRepositoryPath(file.Path) {
		return fmt.Errorf("repository change target %q resolves to protected repository content", target.SymbolID)
	}
	if file.Mode&^uint32(0o777) != 0 {
		return fmt.Errorf("repository change target %q has unsupported file mode %o", target.SymbolID, file.Mode)
	}
	if target.ExpectedFileSHA256 != file.SHA256 || target.StartByte < 0 ||
		target.EndByte <= target.StartByte || target.EndByte > file.Size {
		return fmt.Errorf("repository change target %q is not bound to an exact mutable span", target.SymbolID)
	}
	return nil
}

func rejectOverlappingTargets(replacements []targetReplacement) error {
	ordered := append([]targetReplacement(nil), replacements...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].fileID == ordered[right].fileID {
			if ordered[left].start == ordered[right].start {
				return ordered[left].end < ordered[right].end
			}
			return ordered[left].start < ordered[right].start
		}
		return ordered[left].fileID < ordered[right].fileID
	})
	for index := 1; index < len(ordered); index++ {
		previous, current := ordered[index-1], ordered[index]
		if previous.fileID != current.fileID || current.start >= previous.end {
			continue
		}
		if current.start == previous.start && current.end == previous.end {
			return fmt.Errorf(
				"repository change targets %q and %q have a duplicate range",
				previous.symbolID, current.symbolID,
			)
		}
		return fmt.Errorf(
			"repository change targets %q and %q overlap",
			previous.symbolID, current.symbolID,
		)
	}
	return nil
}

func protectedRepositoryPath(value string) bool {
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == ".git" || part == ".omni" {
			return true
		}
	}
	lower := strings.ToLower(value)
	base := path.Base(lower)
	if base == ".env.example" {
		return false
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	extension := path.Ext(lower)
	return extension == ".pem" || extension == ".key"
}

func joinCleanupError(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	if primary == nil {
		return fmt.Errorf("clean repository change staging workspace: %w", cleanup)
	}
	return errors.Join(primary, fmt.Errorf("clean repository change staging workspace: %w", cleanup))
}
