package worker

import "fmt"

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
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, file.Content); err != nil {
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
