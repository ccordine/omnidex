package worker

import "fmt"

const (
	typeScriptBrowserVersionProfileV1     = "typescript_browser_versions_v1"
	goCommandLineVersionProfileV1         = "go_command_line_versions_v1"
	javaScriptCommandLineVersionProfileV1 = "javascript_command_line_versions_v1"
	rustCommandLineVersionProfileV1       = "rust_command_line_versions_v1"
	javaCommandLineVersionProfileV1       = "java_command_line_versions_v1"
)

const (
	directCodingTypeScriptNodeConstraint = ">=22.20.0 <23.0.0 || >=24.12.0 <25.0.0 || >=25.0.0 <26.0.0"
	directCodingTypeScriptNPMConstraint  = "^10.0.0 || ^11.0.0"
	directCodingJavaScriptNodeConstraint = ">=22.0.0"
	directCodingJavaRelease              = "21"
	directCodingRustEdition              = "2024"
	directCodingRustVersion              = "1.85"
	directCodingGoVersion                = "1.24.0"
)

type directCodingProjectVersionComponent struct {
	Name    string
	Version string
}

type directCodingProjectVersionProfile struct {
	ID         string
	StackID    string
	Components []directCodingProjectVersionComponent
}

func registeredDirectCodingProjectVersionProfiles() []directCodingProjectVersionProfile {
	profiles := []directCodingProjectVersionProfile{
		{
			ID: typeScriptBrowserVersionProfileV1, StackID: genericTypeScriptBrowserAdapter,
			Components: versionComponents(
				"ecmascript", "ES2022", "node", directCodingTypeScriptNodeConstraint,
				"npm", directCodingTypeScriptNPMConstraint,
			),
		},
		{
			ID: goCommandLineVersionProfileV1, StackID: genericGoCommandLineAdapter,
			Components: versionComponents(
				"go", directCodingGoVersion, "go_manifest", ">=1.24.0 <1.25.0",
			),
		},
		{
			ID: javaScriptCommandLineVersionProfileV1, StackID: genericJavaScriptCommandLineAdapter,
			Components: versionComponents(
				"ecmascript", "ES2022", "node", directCodingJavaScriptNodeConstraint,
			),
		},
		{
			ID: rustCommandLineVersionProfileV1, StackID: genericRustCommandLineAdapter,
			Components: versionComponents(
				"cargo_lock", "4", "rust_edition", directCodingRustEdition,
				"rust_manifest", "1.85.0", "rust_version", directCodingRustVersion,
			),
		},
		{
			ID: javaCommandLineVersionProfileV1, StackID: genericJavaCommandLineAdapter,
			Components: versionComponents("java_release", directCodingJavaRelease),
		},
	}
	for index := range profiles {
		profiles[index] = cloneDirectCodingProjectVersionProfile(profiles[index])
	}
	return profiles
}

func directCodingProjectSourceDialect(
	profile directCodingProjectVersionProfile,
) (string, error) {
	component := func(name string) (string, error) {
		return directCodingVersionComponent(profile, name)
	}
	switch profile.StackID {
	case genericTypeScriptBrowserAdapter:
		ecmascript, err := component("ecmascript")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("TypeScript with TSX react-jsx targeting ECMAScript %s", ecmascript), nil
	case genericGoCommandLineAdapter:
		version, err := component("go")
		if err != nil {
			return "", err
		}
		return "Go " + version, nil
	case genericJavaScriptCommandLineAdapter:
		ecmascript, err := component("ecmascript")
		if err != nil {
			return "", err
		}
		node, err := component("node")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ECMAScript %s modules on Node.js %s", ecmascript, node), nil
	case genericRustCommandLineAdapter:
		edition, err := component("rust_edition")
		if err != nil {
			return "", err
		}
		version, err := component("rust_version")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Rust %s edition with rust-version %s", edition, version), nil
	case genericJavaCommandLineAdapter:
		release, err := component("java_release")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Java %s source and class-file API release", release), nil
	default:
		return "", fmt.Errorf("version profile %s has no registered source dialect", profile.ID)
	}
}

func cloneDirectCodingProjectVersionProfile(
	profile directCodingProjectVersionProfile,
) directCodingProjectVersionProfile {
	clone := profile
	clone.Components = append([]directCodingProjectVersionComponent(nil), profile.Components...)
	return clone
}

func versionComponents(values ...string) []directCodingProjectVersionComponent {
	components := make([]directCodingProjectVersionComponent, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		components = append(components, directCodingProjectVersionComponent{Name: values[index], Version: values[index+1]})
	}
	return components
}
