package worker

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
	ID                      string
	StackID                 string
	SourceDialect           string
	Components              []directCodingProjectVersionComponent
	NPMDependencies         map[string]string
	NPMDevDependencies      map[string]string
	NPMLockTemplate         []byte
}

func registeredDirectCodingProjectVersionProfiles() []directCodingProjectVersionProfile {
	profiles := []directCodingProjectVersionProfile{
		{
			ID: typeScriptBrowserVersionProfileV1, StackID: genericTypeScriptBrowserAdapter,
			SourceDialect: "TypeScript 5.9.3 with TSX react-jsx targeting ECMAScript 2022",
			Components: versionComponents(
				"ecmascript", "ES2022", "node", directCodingTypeScriptNodeConstraint,
				"npm", directCodingTypeScriptNPMConstraint, "npm_lock", "3",
				"react", "19.2.7", "tailwindcss", "4.1.12", "typescript", "5.9.3",
				"vite", "6.4.2",
			),
			NPMDependencies: map[string]string{"react": "19.2.7", "react-dom": "19.2.7"},
			NPMDevDependencies: map[string]string{
				"@tailwindcss/vite": "4.1.12", "@types/react": "19.2.17",
				"@types/react-dom": "19.2.3", "@vitejs/plugin-react": "5.2.0",
				"tailwindcss": "4.1.12", "typescript": "5.9.3", "vite": "6.4.2",
			},
			NPMLockTemplate: typeScriptBrowserPackageLockTemplate,
		},
		{
			ID: goCommandLineVersionProfileV1, StackID: genericGoCommandLineAdapter,
			SourceDialect: "Go 1.24.0",
			Components: versionComponents(
				"go", directCodingGoVersion, "go_manifest", ">=1.24.0 <1.25.0",
			),
		},
		{
			ID: javaScriptCommandLineVersionProfileV1, StackID: genericJavaScriptCommandLineAdapter,
			SourceDialect: "ECMAScript 2022 modules on Node.js >=22.0.0",
			Components: versionComponents(
				"ecmascript", "ES2022", "node", directCodingJavaScriptNodeConstraint,
			),
		},
		{
			ID: rustCommandLineVersionProfileV1, StackID: genericRustCommandLineAdapter,
			SourceDialect: "Rust 2024 edition with rust-version 1.85",
			Components: versionComponents(
				"cargo_lock", "4", "rust_edition", directCodingRustEdition,
				"rust_manifest", "1.85.0", "rust_version", directCodingRustVersion,
			),
		},
		{
			ID: javaCommandLineVersionProfileV1, StackID: genericJavaCommandLineAdapter,
			SourceDialect: "Java 21 source and class-file API release",
			Components: versionComponents("java_release", directCodingJavaRelease),
		},
	}
	for index := range profiles {
		profiles[index] = cloneDirectCodingProjectVersionProfile(profiles[index])
	}
	return profiles
}

func cloneDirectCodingProjectVersionProfile(
	profile directCodingProjectVersionProfile,
) directCodingProjectVersionProfile {
	clone := profile
	clone.Components = append([]directCodingProjectVersionComponent(nil), profile.Components...)
	clone.NPMDependencies = cloneStringMap(profile.NPMDependencies)
	clone.NPMDevDependencies = cloneStringMap(profile.NPMDevDependencies)
	clone.NPMLockTemplate = append([]byte(nil), profile.NPMLockTemplate...)
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func versionComponents(values ...string) []directCodingProjectVersionComponent {
	components := make([]directCodingProjectVersionComponent, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		components = append(components, directCodingProjectVersionComponent{Name: values[index], Version: values[index+1]})
	}
	return components
}
