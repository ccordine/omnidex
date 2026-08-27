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

// AssembleExistingGoFileStates validates bounded declaration candidates against
// one exact change contract, then deterministically splices them into complete
// desired file postimages. It performs no staging or filesystem mutation.
func AssembleExistingGoFileStates(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
) ([]DesiredFileState, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("assemble repository desired states snapshot: %w", err)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return nil, fmt.Errorf("assemble repository desired states analysis: %w", err)
	}
	if err := contract.Validate(snapshot, analysis); err != nil {
		return nil, fmt.Errorf("assemble repository desired states contract: %w", err)
	}
	if analysis.Adapter.Name != golangadapter.AdapterName {
		return nil, fmt.Errorf(
			"repository change staging has no final declaration validator for adapter %q",
			analysis.Adapter.Name,
		)
	}
	if len(candidates) != len(contract.Targets) {
		return nil, fmt.Errorf(
			"repository change candidates have %d declarations for %d exact targets",
			len(candidates), len(contract.Targets),
		)
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	replacements := make([]targetReplacement, 0, len(contract.Targets))
	for _, target := range contract.Targets {
		candidate, exists := candidates[target.SymbolID]
		if !exists {
			return nil, fmt.Errorf("repository change target %q has no candidate declaration", target.SymbolID)
		}
		if err := validateCandidate(target.SymbolID, candidate); err != nil {
			return nil, err
		}
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
			snapshot, symbol, maxCandidateDeclarationBytes,
		)
		if err != nil {
			return nil, err
		}
		permitted := target.PermittedCapabilitySymbols()
		fragmentContract := gofragment.Contract{
			Signature: target.Signature, Current: current.Content,
			PermittedSymbols: permitted,
		}
		declaration, err := gofragment.ParseFunction(fragmentContract, candidate)
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
			declaration: []byte(candidate),
		})
	}
	if err := rejectOverlappingTargets(replacements); err != nil {
		return nil, err
	}
	return assembleModifiedFileStates(snapshot, analysis, replacements)
}

func validateCandidate(symbolID, declaration string) error {
	raw := []byte(declaration)
	if len(raw) == 0 || strings.TrimSpace(declaration) == "" {
		return fmt.Errorf("repository change candidate for symbol %q must be non-empty", symbolID)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("repository change candidate for symbol %q must be valid UTF-8", symbolID)
	}
	if strings.ContainsRune(declaration, '\x00') {
		return fmt.Errorf("repository change candidate for symbol %q must be NUL-free", symbolID)
	}
	if len(raw) > maxCandidateDeclarationBytes {
		return fmt.Errorf(
			"repository change candidate for symbol %q exceeds %d bytes",
			symbolID, maxCandidateDeclarationBytes,
		)
	}
	if declaration != strings.TrimSpace(declaration) {
		return fmt.Errorf("repository change candidate for symbol %q must be trimmed", symbolID)
	}
	if strings.ContainsRune(declaration, '\r') {
		return fmt.Errorf("repository change candidate for symbol %q contains unsupported carriage-return bytes", symbolID)
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
