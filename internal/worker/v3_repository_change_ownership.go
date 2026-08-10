package worker

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"

	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type repositoryGoTargetOwnership struct {
	targetSymbolID string
	filePath       string
	startLine      int
	endLine        int
}

type repositoryGoCorrectionOwnership struct {
	stagedRoot string
	targets    []repositoryGoTargetOwnership
}

type stagedGoFile struct {
	set          *token.FileSet
	parsed       *ast.File
	content      []byte
	relativePath string
}

func buildRepositoryGoCorrectionOwnership(
	snapshot repositoryfacts.Snapshot,
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
	stagedRoot string,
) (repositoryGoCorrectionOwnership, error) {
	if err := snapshot.Validate(); err != nil {
		return repositoryGoCorrectionOwnership{}, fmt.Errorf("build staged Go ownership snapshot: %w", err)
	}
	if filepath.Clean(stagedRoot) == "." || !filepath.IsAbs(stagedRoot) {
		return repositoryGoCorrectionOwnership{}, fmt.Errorf("staged Go ownership requires one absolute workspace")
	}
	if _, err := exactRepositoryCandidateDeclarations(contract, candidates); err != nil {
		return repositoryGoCorrectionOwnership{}, err
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	parsedFiles := make(map[string]stagedGoFile)
	owners := make([]repositoryGoTargetOwnership, 0, len(contract.Targets))
	for _, target := range contract.Targets {
		file, exists := files[target.FileID]
		if !exists || file.Kind != repositoryfacts.EntryRegular || file.Language != "go" {
			return repositoryGoCorrectionOwnership{}, fmt.Errorf(
				"repository correction target %q has no exact regular Go file", target.SymbolID,
			)
		}
		parsed, exists := parsedFiles[file.ID]
		if !exists {
			var err error
			parsed, err = parseStagedGoFile(stagedRoot, file.Path)
			if err != nil {
				return repositoryGoCorrectionOwnership{}, err
			}
			parsedFiles[file.ID] = parsed
		}
		owner, err := locateStagedGoTarget(snapshot, target, candidates[target.SymbolID], parsed)
		if err != nil {
			return repositoryGoCorrectionOwnership{}, err
		}
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(left, right int) bool {
		if owners[left].filePath == owners[right].filePath {
			return owners[left].startLine < owners[right].startLine
		}
		return owners[left].filePath < owners[right].filePath
	})
	return repositoryGoCorrectionOwnership{stagedRoot: filepath.Clean(stagedRoot), targets: owners}, nil
}

func parseStagedGoFile(root, relative string) (stagedGoFile, error) {
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	within, err := filepath.Rel(root, absolute)
	if err != nil || within == ".." || filepath.IsAbs(within) ||
		len(within) >= 3 && within[:3] == ".."+string(filepath.Separator) {
		return stagedGoFile{}, fmt.Errorf("staged Go target path escaped its workspace authority")
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return stagedGoFile{}, fmt.Errorf("read staged Go target file: %w", err)
	}
	set := token.NewFileSet()
	parsed, err := parser.ParseFile(set, absolute, content, parser.AllErrors)
	if err != nil {
		return stagedGoFile{}, fmt.Errorf("parse complete staged Go target file: %w", err)
	}
	return stagedGoFile{set: set, parsed: parsed, content: content, relativePath: filepath.ToSlash(relative)}, nil
}

func locateStagedGoTarget(
	snapshot repositoryfacts.Snapshot,
	target repositoryfacts.ChangeTarget,
	candidate string,
	file stagedGoFile,
) (repositoryGoTargetOwnership, error) {
	span, err := repositoryfacts.ReadExactSymbolSpan(
		snapshot, repositoryfacts.Symbol{
			ID: target.SymbolID, FileID: target.FileID, Kind: target.Kind,
			StartByte: target.StartByte, EndByte: target.EndByte,
			SourceSHA256: target.ExpectedFileSHA256,
		}, int(target.EndByte-target.StartByte),
	)
	if err != nil {
		return repositoryGoTargetOwnership{}, err
	}
	permitted := make([]string, len(target.DirectCapabilities))
	for index, capability := range target.DirectCapabilities {
		permitted[index] = capability.Name
	}
	fragmentContract := gofragment.Contract{
		Signature: target.Signature, Current: span.Content, PermittedSymbols: permitted,
	}
	canonical, err := gofragment.ParseFunction(fragmentContract, candidate)
	if err != nil {
		return repositoryGoTargetOwnership{}, fmt.Errorf(
			"validate staged Go correction target %q candidate: %w", target.SymbolID, err,
		)
	}
	matches := make([]repositoryGoTargetOwnership, 0, 1)
	for _, declaration := range file.parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := file.set.PositionFor(function.Pos(), false)
		end := file.set.PositionFor(function.End(), false)
		if start.Offset < 0 || end.Offset <= start.Offset || end.Offset > len(file.content) {
			return repositoryGoTargetOwnership{}, fmt.Errorf("staged Go parser returned an invalid declaration span")
		}
		parsed, parseErr := gofragment.ParseFunction(
			fragmentContract, string(file.content[start.Offset:end.Offset]),
		)
		if parseErr != nil || parsed != canonical {
			continue
		}
		matches = append(matches, repositoryGoTargetOwnership{
			targetSymbolID: target.SymbolID, filePath: file.relativePath,
			startLine: start.Line, endLine: end.Line,
		})
	}
	if len(matches) == 0 {
		return repositoryGoTargetOwnership{}, fmt.Errorf(
			"staged Go correction target %q is absent from its exact parsed file", target.SymbolID,
		)
	}
	if len(matches) != 1 {
		return repositoryGoTargetOwnership{}, fmt.Errorf(
			"staged Go correction target %q is ambiguous in its exact parsed file", target.SymbolID,
		)
	}
	return matches[0], nil
}
