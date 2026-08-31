package worker

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func (s *directCodingSession) hostVerificationAuthority(
	requested directCodingAssembly,
) (*directCodingProgram, directCodingAssembly, bool, error) {
	if s == nil || s.program == nil ||
		s.program.Project.Stack.ID != genericTypeScriptBrowserAdapter {
		return nil, directCodingAssembly{}, false, nil
	}
	complete, err := directCodingProgramHasCompleteGeneratedSet(*s.program)
	if err != nil {
		return nil, directCodingAssembly{}, false, err
	}
	if !complete {
		return nil, directCodingAssembly{}, false, fmt.Errorf(
			"TypeScript host verification requires one complete generated source set",
		)
	}
	assembly, err := directCodingAssemblyFromProgram(*s.program)
	if err != nil {
		return nil, directCodingAssembly{}, false, err
	}
	assembly.RequiredPaths = append([]string(nil), s.program.RequiredPaths...)
	if err := validateDirectCodingProgramAssembly(*s.program, assembly); err != nil {
		return nil, directCodingAssembly{}, false, fmt.Errorf(
			"repeat complete assembly validation at authoritative write gate: %w", err,
		)
	}
	if !directCodingAssembliesEqual(requested, assembly) {
		return nil, directCodingAssembly{}, false, fmt.Errorf(
			"prepared workspace assembly differs from the parser-validated complete program",
		)
	}
	return s.program, assembly, true, nil
}

func directCodingProgramHasCompleteGeneratedSet(
	program directCodingProgram,
) (bool, error) {
	expected := make(map[string]struct{})
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Generated() {
				expected[block.ID] = struct{}{}
			}
		}
	}
	for blockID, source := range program.Generated {
		if _, exists := expected[blockID]; !exists {
			return false, fmt.Errorf("compiled program generated unknown source block %s", blockID)
		}
		if strings.TrimSpace(source) == "" {
			return false, fmt.Errorf("compiled program generated empty source block %s", blockID)
		}
	}
	if len(program.Generated) != len(expected) {
		return false, nil
	}
	return true, nil
}

func directCodingAssembliesEqual(left directCodingAssembly, right directCodingAssembly) bool {
	leftFiles := append([]directCodingFileTask(nil), left.Files...)
	rightFiles := append([]directCodingFileTask(nil), right.Files...)
	sort.Slice(leftFiles, func(i, j int) bool { return leftFiles[i].Path < leftFiles[j].Path })
	sort.Slice(rightFiles, func(i, j int) bool { return rightFiles[i].Path < rightFiles[j].Path })
	if len(leftFiles) != len(rightFiles) {
		return false
	}
	for index := range leftFiles {
		first, second := leftFiles[index], rightFiles[index]
		if first.Path != second.Path || first.Mode != second.Mode ||
			first.MoveFrom != second.MoveFrom || !bytes.Equal(first.Content, second.Content) {
			return false
		}
	}
	return directCodingStringSetsEqual(left.RequiredPaths, right.RequiredPaths) &&
		directCodingStringSetsEqual(left.DeletePaths, right.DeletePaths)
}

func directCodingStringSetsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	first := append([]string(nil), left...)
	second := append([]string(nil), right...)
	sort.Strings(first)
	sort.Strings(second)
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
