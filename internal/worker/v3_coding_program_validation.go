package worker

import (
	"fmt"
	"strings"
)

func validateDirectCodingProgramAssembly(
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	stack := program.Project.Stack
	profile := program.Project.Profile
	if stack.ID == "" || profile.ID == "" || profile.StackID != stack.ID {
		return fmt.Errorf("compiled program lacks one immutable stack/version-profile binding")
	}
	if err := program.Source.Validate(); err != nil {
		return fmt.Errorf("validate source blueprint before assembly: %w", err)
	}
	if stack.ValidateBlueprint == nil {
		return fmt.Errorf("project stack %s has no blueprint validator", stack.ID)
	}
	if err := stack.ValidateBlueprint(program.Source); err != nil {
		return fmt.Errorf("project stack %s rejected its source blueprint: %w", stack.ID, err)
	}
	if stack.ValidateSourceOwnership == nil {
		return fmt.Errorf("project stack %s has no source-ownership validator", stack.ID)
	}
	if err := stack.ValidateSourceOwnership(program.Workload, program.Source); err != nil {
		return fmt.Errorf("project stack %s rejected source ownership: %w", stack.ID, err)
	}
	if err := program.RequirementRelations.validateCompleteFor(program.Workload); err != nil {
		return err
	}
	files := make(map[string]directCodingFileTask, len(assembly.Files))
	for _, file := range assembly.Files {
		if _, err := requireExactDirectCodingPath(file.Path); err != nil {
			return err
		}
		if _, duplicate := files[file.Path]; duplicate {
			return fmt.Errorf("compiled assembly repeats source path %q", file.Path)
		}
		if file.Mode&^uint32(0o777) != 0 || file.Mode == 0 {
			return fmt.Errorf("compiled assembly path %q has invalid permission mode", file.Path)
		}
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, file.Content); err != nil {
			return err
		}
		files[file.Path] = file
	}
	for _, targetPath := range program.TargetTree.Paths {
		if _, exists := files[targetPath]; !exists {
			return fmt.Errorf("compiled target path %q is absent from the validated assembly", targetPath)
		}
	}
	if stack.ID == genericTypeScriptBrowserAdapter {
		if err := validateTypeScriptBrowserAssembly(assembly); err != nil {
			return err
		}
		manifest := files["package.json"]
		lock := files["package-lock.json"]
		if len(manifest.Content) == 0 || len(lock.Content) == 0 {
			return fmt.Errorf("TypeScript browser assembly lacks exact package authority")
		}
		if err := validatePinnedNPMLockForProfile(
			string(manifest.Content), string(lock.Content), profile,
		); err != nil {
			return fmt.Errorf("TypeScript browser version-profile dependency authority: %w", err)
		}
	}
	return nil
}

func validateDirectCodingProjectedProgramAssembly(
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	stack := program.Project.Stack
	profile := program.Project.Profile
	if stack.ID == "" || profile.ID == "" || profile.StackID != stack.ID {
		return fmt.Errorf("projected program lacks one immutable stack/version-profile binding")
	}
	if err := program.Source.Validate(); err != nil {
		return fmt.Errorf("validate projected source blueprint: %w", err)
	}
	if stack.ValidateBlueprint == nil {
		return fmt.Errorf("project stack %s has no blueprint validator", stack.ID)
	}
	if err := stack.ValidateBlueprint(program.Source); err != nil {
		return fmt.Errorf("project stack %s rejected projected source: %w", stack.ID, err)
	}
	files := make(map[string]struct{}, len(assembly.Files))
	for _, file := range assembly.Files {
		if _, duplicate := files[file.Path]; duplicate {
			return fmt.Errorf("projected assembly repeats source path %q", file.Path)
		}
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, file.Content); err != nil {
			return err
		}
		files[file.Path] = struct{}{}
	}
	for _, targetPath := range program.TargetTree.Paths {
		if _, exists := files[targetPath]; !exists {
			return fmt.Errorf("projected target path %q is absent from the validated assembly", targetPath)
		}
	}
	return nil
}

func validateDirectCodingStackArtifactSource(
	stack directCodingProjectStack,
	artifactPath string,
	content []byte,
) error {
	adapter, _, err := directCodingArtifactAdapterForProjectPath(stack, artifactPath)
	if err != nil {
		return err
	}
	if err := validateDirectCodingArtifactSource(adapter, artifactPath, content); err != nil {
		return err
	}
	return nil
}

func directCodingAssemblyFile(
	assembly directCodingAssembly,
	artifactPath string,
) (directCodingFileTask, error) {
	var found *directCodingFileTask
	for index := range assembly.Files {
		if assembly.Files[index].Path != artifactPath {
			continue
		}
		if found != nil {
			return directCodingFileTask{}, fmt.Errorf("compiled assembly repeats source path %q", artifactPath)
		}
		candidate := assembly.Files[index]
		found = &candidate
	}
	if found == nil || strings.TrimSpace(string(found.Content)) == "" {
		return directCodingFileTask{}, fmt.Errorf("compiled assembly requires one non-empty %s", artifactPath)
	}
	return *found, nil
}
