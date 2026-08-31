package worker

import (
	"fmt"
)

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

type directCodingArtifactVersion struct {
	AdapterID string
	Version   string
}

type directCodingProjectVersionComponent struct {
	Name    string
	Version string
}

type directCodingProjectVersionProfile struct {
	ID                      string
	StackID                 string
	SourceDialect           string
	ParserQualification     string
	ManifestPaths           []string
	ArtifactVersions        []directCodingArtifactVersion
	Components              []directCodingProjectVersionComponent
	NPMDependencies         map[string]string
	NPMDevDependencies      map[string]string
	NPMLockTemplate         []byte
	ValidateAssembly        func(directCodingProjectVersionProfile, directCodingProgram, directCodingAssembly) error
	ValidateDefinition      func(directCodingProjectVersionProfile) error
}

func registeredDirectCodingProjectVersionProfiles() []directCodingProjectVersionProfile {
	profiles := []directCodingProjectVersionProfile{
		{
			ID: typeScriptBrowserVersionProfileV1, StackID: genericTypeScriptBrowserAdapter,
			SourceDialect:       "TypeScript 5.9.3 with TSX react-jsx targeting ECMAScript 2022",
			ParserQualification: "tree-sitter-typescript-0.23.2-typescript-5.9-profile-v1",
			ManifestPaths:       []string{"package.json"},
			ArtifactVersions: artifactVersions(
				"css_tailwind", "Tailwind CSS 4.1.12", "html", "HTML5 qualified subset v1",
				"plain_text", "UTF-8 text profile v1", "structured_json", "RFC 8259",
				"typescript", "TypeScript 5.9.3", "typescript_react", "TypeScript 5.9.3 and React 19.2.7",
			),
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
			NPMLockTemplate:    typeScriptBrowserPackageLockTemplate,
			ValidateAssembly:   validateTypeScriptBrowserVersionProfileAssembly,
			ValidateDefinition: validateTypeScriptBrowserVersionProfile,
		},
		{
			ID: goCommandLineVersionProfileV1, StackID: genericGoCommandLineAdapter,
			SourceDialect: "Go 1.24.0", ParserQualification: "go-parser-go1.24-profile-v1",
			ManifestPaths: []string{"go.mod"},
			ArtifactVersions: artifactVersions(
				"go", "Go 1.24.0", "go_module", "Go module 1.24.0", "plain_text", "UTF-8 text profile v1",
			),
			Components: versionComponents(
				"go", directCodingGoVersion, "go_manifest", ">=1.24.0 <1.25.0",
			),
			ValidateAssembly:   validateGoVersionProfileAssembly,
			ValidateDefinition: validateGoVersionProfile,
		},
		{
			ID: javaScriptCommandLineVersionProfileV1, StackID: genericJavaScriptCommandLineAdapter,
			SourceDialect:       "ECMAScript 2022 modules on Node.js >=22.0.0",
			ParserQualification: "tree-sitter-javascript-0.23.1-es2022-profile-v1",
			ManifestPaths:       []string{"package.json"},
			ArtifactVersions: artifactVersions(
				"javascript", "ECMAScript 2022 modules", "plain_text", "UTF-8 text profile v1",
				"structured_json", "RFC 8259",
			),
			Components: versionComponents(
				"ecmascript", "ES2022", "node", directCodingJavaScriptNodeConstraint,
			),
			ValidateAssembly:   validateJavaScriptVersionProfileAssembly,
			ValidateDefinition: validateJavaScriptVersionProfile,
		},
		{
			ID: rustCommandLineVersionProfileV1, StackID: genericRustCommandLineAdapter,
			SourceDialect:       "Rust 2024 edition with rust-version 1.85",
			ParserQualification: "tree-sitter-rust-0.23.2-edition-2024-profile-v1",
			ManifestPaths:       []string{"Cargo.toml"},
			ArtifactVersions: artifactVersions(
				"cargo_toml", "Cargo lock format 4", "plain_text", "UTF-8 text profile v1",
				"rust", "Rust 2024 edition",
			),
			Components: versionComponents(
				"cargo_lock", "4", "rust_edition", directCodingRustEdition,
				"rust_manifest", "1.85.0", "rust_version", directCodingRustVersion,
			),
			ValidateAssembly:   validateRustVersionProfileAssembly,
			ValidateDefinition: validateRustVersionProfile,
		},
		{
			ID: javaCommandLineVersionProfileV1, StackID: genericJavaCommandLineAdapter,
			SourceDialect:       "Java 21 source and class-file API release",
			ParserQualification: "tree-sitter-java-0.23.5-release-21-profile-v1",
			ArtifactVersions: artifactVersions(
				"java", "Java release 21", "plain_text", "UTF-8 text profile v1",
			),
			Components:         versionComponents("java_release", directCodingJavaRelease),
			ValidateAssembly:   validateJavaVersionProfileAssembly,
			ValidateDefinition: validateJavaVersionProfile,
		},
	}
	for index := range profiles {
		profiles[index] = cloneDirectCodingProjectVersionProfile(profiles[index])
	}
	return profiles
}

func directCodingProjectVersionProfileByID(id string) (directCodingProjectVersionProfile, error) {
	profile, exists := directCodingRegisteredProjectVersionProfileByID(id)
	if exists {
		return cloneDirectCodingProjectVersionProfile(profile), nil
	}
	return directCodingProjectVersionProfile{}, fmt.Errorf("project version profile %q is not registered", id)
}

func directCodingRegisteredProjectVersionProfileByID(
	id string,
) (directCodingProjectVersionProfile, bool) {
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		if profile.ID == id {
			return cloneDirectCodingProjectVersionProfile(profile), true
		}
	}
	return directCodingProjectVersionProfile{}, false
}

func cloneDirectCodingProjectVersionProfile(
	profile directCodingProjectVersionProfile,
) directCodingProjectVersionProfile {
	clone := profile
	clone.ManifestPaths = append([]string(nil), profile.ManifestPaths...)
	clone.ArtifactVersions = append([]directCodingArtifactVersion(nil), profile.ArtifactVersions...)
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

func artifactVersions(values ...string) []directCodingArtifactVersion {
	versions := make([]directCodingArtifactVersion, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		versions = append(versions, directCodingArtifactVersion{AdapterID: values[index], Version: values[index+1]})
	}
	return versions
}

func versionComponents(values ...string) []directCodingProjectVersionComponent {
	components := make([]directCodingProjectVersionComponent, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		components = append(components, directCodingProjectVersionComponent{Name: values[index], Version: values[index+1]})
	}
	return components
}
