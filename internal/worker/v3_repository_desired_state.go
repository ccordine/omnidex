package worker

import (
	"fmt"
	"go/format"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type desiredRepositoryCompileResult struct {
	States                  []changeapply.DesiredFileState
	CreatedFiles            int
	DeletedFiles            int
	DeterministicOperations int
}

func compileDesiredRepositoryFileStates(
	graph repositoryfacts.DesiredArtifactGraph,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	candidates map[string]string,
) (desiredRepositoryCompileResult, error) {
	var result desiredRepositoryCompileResult
	if err := graph.Validate(snapshot, analysis); err != nil {
		return result, err
	}
	currentCandidates := make(map[string]string, len(candidates))
	for id, candidate := range candidates {
		currentCandidates[id] = candidate
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	for _, artifact := range graph.Artifacts {
		placement, err := repositoryfacts.ResolveGoPackagePlacement(
			snapshot, analysis, artifact.PackageArtifactID,
		)
		if err != nil {
			return result, err
		}
		if artifact.MustExist {
			if len(artifact.ExistingSymbolIDs) != 0 {
				return result, fmt.Errorf(
					"desired artifact %q already has source authority and requires ordinary bounded modification",
					artifact.ID,
				)
			}
			candidate, exists := currentCandidates[artifact.ID]
			if !exists {
				return result, fmt.Errorf("desired artifact %q has no bounded declaration candidate", artifact.ID)
			}
			delete(currentCandidates, artifact.ID)
			state, err := compileDesiredCreatedGoFile(snapshot, placement, artifact, candidate)
			if err != nil {
				return result, err
			}
			result.States = append(result.States, state)
			result.CreatedFiles++
			result.DeterministicOperations++
			continue
		}
		if len(artifact.ExistingSymbolIDs) == 0 {
			return result, fmt.Errorf("desired absent artifact %q is already absent and requires no mutation", artifact.ID)
		}
		state, err := compileDesiredDeletedGoFile(files, symbols, placement, artifact)
		if err != nil {
			return result, err
		}
		result.States = append(result.States, state)
		result.DeletedFiles++
		result.DeterministicOperations++
	}
	if len(currentCandidates) != 0 {
		ids := make([]string, 0, len(currentCandidates))
		for id := range currentCandidates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return result, fmt.Errorf("desired artifact candidates contain unowned declaration %q", ids[0])
	}
	sort.Slice(result.States, func(left, right int) bool {
		return result.States[left].Path < result.States[right].Path
	})
	return result, nil
}

func compileDesiredCreatedGoFile(
	snapshot repositoryfacts.Snapshot,
	placement repositoryfacts.GoPackagePlacement,
	artifact repositoryfacts.DesiredGoArtifact,
	candidate string,
) (changeapply.DesiredFileState, error) {
	compiled, err := gofragment.CompileNewFunctionSignature(artifact.Signature)
	if err != nil {
		return changeapply.DesiredFileState{}, err
	}
	declaration, err := gofragment.ParseNewFunction(compiled.Canonical, nil, candidate)
	if err != nil {
		return changeapply.DesiredFileState{}, fmt.Errorf("desired artifact %q declaration: %w", artifact.ID, err)
	}
	filename, err := deterministicGoSourceName(compiled.Name)
	if err != nil {
		return changeapply.DesiredFileState{}, err
	}
	target := filename
	if placement.Directory != "." {
		target = path.Join(placement.Directory, filename)
	}
	if _, err := repositoryfacts.FileIDForAbsentPath(snapshot, target); err != nil {
		return changeapply.DesiredFileState{}, err
	}
	content, err := format.Source([]byte("package " + placement.Name + "\n\n" + declaration + "\n"))
	if err != nil {
		return changeapply.DesiredFileState{}, fmt.Errorf("assemble desired Go source: %w", err)
	}
	return changeapply.DesiredFileState{
		Path: target, Present: true, Content: content, Mode: 0o644,
		PackageArtifactID: artifact.PackageArtifactID,
	}, nil
}

func compileDesiredDeletedGoFile(
	files map[string]repositoryfacts.File,
	symbols map[string]repositoryfacts.Symbol,
	placement repositoryfacts.GoPackagePlacement,
	artifact repositoryfacts.DesiredGoArtifact,
) (changeapply.DesiredFileState, error) {
	var file repositoryfacts.File
	for _, symbolID := range artifact.ExistingSymbolIDs {
		symbol, exists := symbols[symbolID]
		if !exists {
			return changeapply.DesiredFileState{}, fmt.Errorf("desired absent artifact references unknown symbol %q", symbolID)
		}
		current, exists := files[symbol.FileID]
		if !exists || path.Dir(current.Path) != placement.Directory {
			return changeapply.DesiredFileState{}, fmt.Errorf("desired absent artifact symbol %q has invalid package ownership", symbolID)
		}
		if file.ID == "" {
			file = current
		} else if file.ID != current.ID {
			return changeapply.DesiredFileState{}, fmt.Errorf("desired absent artifact spans multiple files and is unsupported")
		}
	}
	return changeapply.DesiredFileState{
		Path: file.Path, Present: false,
		Source: changeapply.ExactSourceFile{
			FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
		},
		RemovedSymbolIDs: append([]string(nil), artifact.ExistingSymbolIDs...),
	}, nil
}

func deterministicGoSourceName(name string) (string, error) {
	if name == "" || !utf8.ValidString(name) || name == "init" {
		return "", fmt.Errorf("new Go declaration has unsupported code-owned name %q", name)
	}
	runes := []rune(name)
	var output strings.Builder
	for index, current := range runes {
		if current > unicode.MaxASCII || !unicode.IsLetter(current) && !unicode.IsDigit(current) {
			return "", fmt.Errorf("new Go declaration name %q cannot be mapped to a portable source name", name)
		}
		if unicode.IsUpper(current) && index > 0 && (unicode.IsLower(runes[index-1]) ||
			unicode.IsDigit(runes[index-1]) || index+1 < len(runes) && unicode.IsLower(runes[index+1])) {
			output.WriteByte('_')
		}
		output.WriteRune(unicode.ToLower(current))
	}
	if output.Len() == 0 {
		return "", fmt.Errorf("new Go declaration has no portable source name")
	}
	stem := output.String()
	// A neutral suffix prevents declaration names from accidentally selecting
	// Go's _test or GOOS/GOARCH filename semantics.
	return "omni_" + stem + "_artifact.go", nil
}
