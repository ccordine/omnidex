package worker

import (
	"fmt"
	"strings"
)

func validateDirectCodingArtifactRegistries() error {
	return validateDirectCodingArtifactRegistriesFrom(
		registeredDirectCodingArtifactAdapters(),
		registeredDirectCodingProjectVersionProfiles(),
		registeredDirectCodingProjectStacks(),
		registeredDirectCodingParserQualifications(),
	)
}

func validateDirectCodingArtifactRegistriesFrom(
	adapters []directCodingArtifactAdapter,
	profiles []directCodingProjectVersionProfile,
	stacks []directCodingProjectStack,
	qualifications []directCodingParserQualification,
) error {
	adapterIDs := make(map[string]struct{}, len(adapters))
	adapterByID := make(map[string]directCodingArtifactAdapter, len(adapters))
	for index, adapter := range adapters {
		if adapter.ID == "" || adapter.ID != strings.TrimSpace(adapter.ID) {
			return fmt.Errorf("artifact adapter %d has an invalid ID", index)
		}
		if _, duplicate := adapterIDs[adapter.ID]; duplicate {
			return fmt.Errorf("artifact adapter ID %q is registered more than once", adapter.ID)
		}
		adapterIDs[adapter.ID] = struct{}{}
		adapterByID[adapter.ID] = adapter
		if adapter.Recognize == nil || adapter.Validation.Execute == nil {
			return fmt.Errorf("artifact adapter %s requires path recognition and executable source validation", adapter.ID)
		}
		switch adapter.Validation.Kind {
		case directCodingArtifactParse, directCodingArtifactStructural:
		default:
			return fmt.Errorf(
				"artifact adapter %s has unknown executable validation kind %q",
				adapter.ID, adapter.Validation.Kind,
			)
		}
	}
	profileIDs := make(map[string]directCodingProjectVersionProfile, len(profiles))
	for index, profile := range profiles {
		if profile.ID == "" || profile.ID != strings.TrimSpace(profile.ID) {
			return fmt.Errorf("project version profile %d has an invalid ID", index)
		}
		if _, duplicate := profileIDs[profile.ID]; duplicate {
			return fmt.Errorf("project version profile ID %q is registered more than once", profile.ID)
		}
		if strings.TrimSpace(profile.StackID) == "" ||
			strings.TrimSpace(profile.SourceDialect) == "" ||
			strings.TrimSpace(profile.ParserQualification) == "" ||
			profile.MatchExisting == nil || profile.ValidateRuntime == nil || profile.ValidateAssembly == nil ||
			profile.ValidateDefinition == nil {
			return fmt.Errorf(
				"project version profile %s requires stack, dialect, parser, compatibility, qualification, and assembly authority",
				profile.ID,
			)
		}
		if err := validateDirectCodingVersionProfilePaths(profile); err != nil {
			return err
		}
		if err := validateDirectCodingVersionProfileValues(profile); err != nil {
			return err
		}
		if err := profile.ValidateDefinition(profile); err != nil {
			return fmt.Errorf("qualify project version profile %s: %w", profile.ID, err)
		}
		profileIDs[profile.ID] = profile
	}
	if err := validateDirectCodingParserQualificationsFrom(adapterByID, profiles, qualifications); err != nil {
		return err
	}
	stackIDs := make(map[string]struct{})
	constraintDescriptions := make(map[string]struct{})
	for index, stack := range stacks {
		if stack.ID == "" || stack.ID != strings.TrimSpace(stack.ID) {
			return fmt.Errorf("project stack %d has an invalid ID", index)
		}
		if _, duplicate := stackIDs[stack.ID]; duplicate {
			return fmt.Errorf("project stack ID %q is registered more than once", stack.ID)
		}
		stackIDs[stack.ID] = struct{}{}
		defaultProfile, exists := profileIDs[stack.DefaultVersionProfileID]
		if !exists || defaultProfile.StackID != stack.ID {
			return fmt.Errorf(
				"project stack %s references unsupported default version profile %q",
				stack.ID, stack.DefaultVersionProfileID,
			)
		}
		if stack.ConstraintDescription == "" || stack.ConstraintDescription != strings.TrimSpace(stack.ConstraintDescription) ||
			strings.ContainsAny(stack.ConstraintDescription, "\x00\r\n") || len(stack.ConstraintDescription) > 256 {
			return fmt.Errorf("project stack %s has an invalid constraint description", stack.ID)
		}
		if _, duplicate := constraintDescriptions[stack.ConstraintDescription]; duplicate {
			return fmt.Errorf("project stack %s repeats a constraint description", stack.ID)
		}
		constraintDescriptions[stack.ConstraintDescription] = struct{}{}
		if err := validateDirectCodingProjectStackSurfaceSet(stack); err != nil {
			return err
		}
		compilerCount := 0
		if stack.CompileSource != nil {
			compilerCount++
		}
		if stack.CompileServiceSource != nil {
			compilerCount++
		}
		if (stack.CompileServiceSource != nil) != (stack.ValidateServiceState != nil) {
			return fmt.Errorf(
				"project stack %s must bind its HTTP compiler and state-lifetime validator together",
				stack.ID,
			)
		}
		if (stack.RuntimeCapabilities != nil) != (stack.BindRuntimeCapabilities != nil) {
			return fmt.Errorf(
				"project stack %s must bind runtime capability registry and source projection together",
				stack.ID,
			)
		}
		if stack.RuntimeCapabilities != nil {
			capabilities, err := stack.RuntimeCapabilities()
			if err != nil {
				return fmt.Errorf("project stack %s runtime capability registry: %w", stack.ID, err)
			}
			if err := validateDirectCodingRuntimeCapabilityRegistry(capabilities); err != nil {
				return fmt.Errorf("project stack %s runtime capability registry: %w", stack.ID, err)
			}
		}
		if stack.Deployment != nil {
			if stack.CompileServiceSource == nil {
				return fmt.Errorf("project stack %s cannot deploy without an HTTP compiler", stack.ID)
			}
			if err := stack.Deployment.validate(); err != nil {
				return fmt.Errorf("project stack %s deployment descriptor: %w", stack.ID, err)
			}
		}
		if stack.ProjectCompleteTargetTree != nil && stack.ProjectFocusedTargetTree != nil {
			return fmt.Errorf(
				"project stack %s cannot register both complete and focused target-tree projectors",
				stack.ID,
			)
		}
		if strings.TrimSpace(stack.TreeDescription) == "" || len(stack.ArtifactAdapterIDs) == 0 ||
			len(stack.TargetTreeAdapterIDs) == 0 || len(stack.TaskStageStaticPaths) == 0 ||
			compilerCount != 1 || stack.ValidateTargetTree == nil || stack.ValidateBlueprint == nil ||
			stack.ValidateSourceOwnership == nil ||
			stack.ValidateAssembly == nil || stack.VerificationCommands == nil || stack.NewStageExecutor == nil {
			return fmt.Errorf(
				"project stack %s requires technical context, executable assembly and verification, and artifact registries",
				stack.ID,
			)
		}
		for cleanupIndex, command := range stack.CleanupCommands {
			if err := validateV3Command(command.Name, command.Args); err != nil {
				return fmt.Errorf(
					"project stack %s cleanup command %d is outside the code-owned boundary: %w",
					stack.ID, cleanupIndex, err,
				)
			}
			if command.Timeout <= 0 || command.Timeout > maxV3CommandLimit {
				return fmt.Errorf(
					"project stack %s cleanup command %d has invalid timeout %s",
					stack.ID, cleanupIndex, command.Timeout,
				)
			}
		}
		allowed := make(map[string]struct{}, len(stack.ArtifactAdapterIDs))
		for _, adapterID := range stack.ArtifactAdapterIDs {
			if _, exists := adapterIDs[adapterID]; !exists {
				return fmt.Errorf("project stack %s references unknown artifact adapter %q", stack.ID, adapterID)
			}
			if _, duplicate := allowed[adapterID]; duplicate {
				return fmt.Errorf("project stack %s repeats artifact adapter %q", stack.ID, adapterID)
			}
			allowed[adapterID] = struct{}{}
		}
		profileCount := 0
		for _, profile := range profiles {
			if profile.StackID != stack.ID {
				continue
			}
			profileCount++
			if len(profile.ArtifactVersions) != len(allowed) {
				return fmt.Errorf(
					"project version profile %s covers artifacts=%d want=%d",
					profile.ID, len(profile.ArtifactVersions), len(allowed),
				)
			}
			for _, version := range profile.ArtifactVersions {
				if _, exists := allowed[version.AdapterID]; !exists {
					return fmt.Errorf(
						"project version profile %s versions non-stack adapter %s",
						profile.ID, version.AdapterID,
					)
				}
			}
		}
		if profileCount == 0 {
			return fmt.Errorf("project stack %s has no registered version profiles", stack.ID)
		}
		treeAdapters := make(map[string]struct{}, len(stack.TargetTreeAdapterIDs))
		for _, adapterID := range stack.TargetTreeAdapterIDs {
			if _, exists := allowed[adapterID]; !exists {
				return fmt.Errorf("project stack %s exposes non-member tree adapter %q", stack.ID, adapterID)
			}
			if _, duplicate := treeAdapters[adapterID]; duplicate {
				return fmt.Errorf("project stack %s repeats tree adapter %q", stack.ID, adapterID)
			}
			treeAdapters[adapterID] = struct{}{}
		}
		if err := validateDirectCodingTargetTreeReservedPaths(
			stack, adapterByID, treeAdapters,
		); err != nil {
			return err
		}
		if err := stack.TargetTreeConstraints.Validate(); err != nil {
			return fmt.Errorf("project stack %s target-tree constraints: %w", stack.ID, err)
		}
	}
	for _, profile := range profiles {
		if _, exists := stackIDs[profile.StackID]; !exists {
			return fmt.Errorf(
				"project version profile %s references unknown stack %s", profile.ID, profile.StackID,
			)
		}
	}
	return validateDirectCodingProjectStackDefaults(stacks)
}

func validateDirectCodingVersionProfilePaths(profile directCodingProjectVersionProfile) error {
	seen := make(map[string]struct{}, len(profile.ManifestPaths))
	last := ""
	for index, value := range profile.ManifestPaths {
		normalized, err := normalizeDirectCodingPath(value)
		if err != nil || normalized != value {
			return fmt.Errorf("project version profile %s manifest path %d is not normalized", profile.ID, index)
		}
		if _, duplicate := seen[value]; duplicate || last != "" && value <= last {
			return fmt.Errorf("project version profile %s manifest paths are duplicated or unordered", profile.ID)
		}
		seen[value] = struct{}{}
		last = value
	}
	return nil
}

func validateDirectCodingVersionProfileValues(profile directCodingProjectVersionProfile) error {
	lastArtifact := ""
	for _, value := range profile.ArtifactVersions {
		if strings.TrimSpace(value.AdapterID) == "" || strings.TrimSpace(value.Version) == "" ||
			lastArtifact != "" && value.AdapterID <= lastArtifact {
			return fmt.Errorf("project version profile %s artifact versions are incomplete or unordered", profile.ID)
		}
		lastArtifact = value.AdapterID
	}
	lastComponent := ""
	for _, value := range profile.Components {
		if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Version) == "" ||
			lastComponent != "" && value.Name <= lastComponent {
			return fmt.Errorf("project version profile %s components are incomplete or unordered", profile.ID)
		}
		lastComponent = value.Name
	}
	for name, version := range profile.NPMDependencies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("project version profile %s has an invalid npm dependency", profile.ID)
		}
	}
	for name, version := range profile.ComposerDependencies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("project version profile %s has an invalid Composer dependency", profile.ID)
		}
	}
	for name, version := range profile.ComposerDevDependencies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("project version profile %s has an invalid Composer development dependency", profile.ID)
		}
	}
	hasComposer := len(profile.ComposerDependencies) != 0 || len(profile.ComposerDevDependencies) != 0
	if hasComposer != (len(profile.ComposerLockTemplate) != 0) {
		return fmt.Errorf(
			"project version profile %s must bind Composer dependencies and one lock template together", profile.ID,
		)
	}
	for name, version := range profile.NPMDevDependencies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("project version profile %s has an invalid npm development dependency", profile.ID)
		}
	}
	hasNPM := len(profile.NPMDependencies) != 0 || len(profile.NPMDevDependencies) != 0
	if hasNPM != (len(profile.NPMLockTemplate) != 0) {
		return fmt.Errorf(
			"project version profile %s must bind npm dependencies and one lock template together", profile.ID,
		)
	}
	return nil
}
