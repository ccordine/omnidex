package worker

import (
	"fmt"
)

func (s *directCodingSession) validateProgramSource(path, content string) error {
	if s.program == nil || s.specification == nil {
		return fmt.Errorf("coding program validation requires accepted typed semantics and a compiled adapter")
	}
	return validateDirectCodingProgramSource(path, content, *s.program)
}

func validateDirectCodingProgramSource(path, content string, program directCodingProgram) error {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return err
	}
	expected := ""
	for _, file := range assembly.Files {
		if file.Path == path {
			expected = file.Content
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("adapter %s emitted undeclared source path %s", program.StackID, path)
	}
	if content != expected {
		return fmt.Errorf("adapter %s source %s differs from its parser-validated in-memory authority", program.StackID, path)
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return err
	}
	if err := validateDirectCodingStackArtifactSource(stack, path, []byte(content)); err != nil {
		return err
	}
	if err := validateDirectCodingStackAssembly(stack, assembly); err != nil {
		return err
	}
	return validateDirectCodingProgramVersionProfile(program, assembly)
}

func validateDirectCodingAssemblySources(
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	if err := assembly.validate(); err != nil {
		return err
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return err
	}
	for _, file := range assembly.Files {
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, []byte(file.Content)); err != nil {
			return err
		}
	}
	if err := validateDirectCodingStackAssembly(stack, assembly); err != nil {
		return err
	}
	return validateDirectCodingProgramVersionProfile(program, assembly)
}

func validateDirectCodingStackArtifactSource(
	stack directCodingProjectStack,
	path string,
	content []byte,
) error {
	adapter, _, err := directCodingArtifactAdapterForProjectPath(stack, path)
	if err != nil {
		return err
	}
	if err := validateDirectCodingArtifactSource(adapter, path, content); err != nil {
		return err
	}
	return nil
}

func validateDirectCodingStackAssembly(
	stack directCodingProjectStack,
	assembly directCodingAssembly,
) error {
	if stack.ValidateAssembly == nil {
		return fmt.Errorf("project stack %s has no complete-assembly validator", stack.ID)
	}
	if err := stack.ValidateAssembly(assembly); err != nil {
		return fmt.Errorf("project stack %s rejected its complete assembly: %w", stack.ID, err)
	}
	return nil
}

func validateDirectCodingProgramVersionProfile(
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	if assembly.VersionProfileID != program.VersionProfileID {
		return fmt.Errorf(
			"assembly version profile %q differs from program authority %q",
			assembly.VersionProfileID, program.VersionProfileID,
		)
	}
	profile, err := directCodingVersionProfileForProgram(program)
	if err != nil {
		return err
	}
	if profile.ValidateAssembly == nil {
		return fmt.Errorf("project version profile %s has no assembly validator", profile.ID)
	}
	if err := profile.ValidateAssembly(profile, program, assembly); err != nil {
		return fmt.Errorf("project version profile %s rejected its assembly: %w", profile.ID, err)
	}
	return nil
}
