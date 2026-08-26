package worker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
)

func validateTypeScriptBrowserVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	_ directCodingProgram,
	assembly directCodingAssembly,
) error {
	files := directCodingAssemblyFiles(assembly)
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return err
	}
	if err := validateNPMManifestVersionProfile(
		files["package.json"], profile, map[string]string{"node": node, "npm": npm},
	); err != nil {
		return err
	}
	if err := validatePinnedNPMLockForProfile(
		files["package.json"], files["package-lock.json"], profile,
	); err != nil {
		return err
	}
	ecmascript, err := directCodingVersionComponent(profile, "ecmascript")
	if err != nil {
		return err
	}
	if !strings.Contains(files["tsconfig.json"], `"target": "`+ecmascript+`"`) ||
		!strings.Contains(files["tsconfig.json"], `"lib": ["`+ecmascript+`"`) {
		return fmt.Errorf("TypeScript version profile requires its %s compiler target", ecmascript)
	}
	return nil
}

func validateGoVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	_ directCodingProgram,
	assembly directCodingAssembly,
) error {
	manifest, err := modfile.Parse("go.mod", []byte(directCodingAssemblyFiles(assembly)["go.mod"]), nil)
	if err != nil {
		return err
	}
	version, err := directCodingVersionComponent(profile, "go")
	if err != nil {
		return err
	}
	if manifest.Go == nil || manifest.Go.Version != version {
		return fmt.Errorf("Go version profile requires generated directive %s", version)
	}
	return nil
}

func validateJavaScriptVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	_ directCodingProgram,
	assembly directCodingAssembly,
) error {
	files := directCodingAssemblyFiles(assembly)
	var manifest struct {
		Type    string            `json:"type"`
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal([]byte(files["package.json"]), &manifest); err != nil {
		return err
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	if manifest.Type != "module" || manifest.Engines["node"] != node {
		return fmt.Errorf("JavaScript version profile requires ESM and Node.js %s", node)
	}
	guard, err := javaScriptNodeRuntimeGuard(node)
	if err != nil {
		return err
	}
	if !strings.Contains(files["runtime.mjs"], guard) {
		return fmt.Errorf("JavaScript version profile lacks its Node.js runtime compatibility guard")
	}
	return nil
}

func validateRustVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	_ directCodingProgram,
	assembly directCodingAssembly,
) error {
	var manifest struct {
		Package struct {
			Edition     string `toml:"edition"`
			RustVersion string `toml:"rust-version"`
		} `toml:"package"`
	}
	files := directCodingAssemblyFiles(assembly)
	if err := toml.Unmarshal([]byte(files["Cargo.toml"]), &manifest); err != nil {
		return err
	}
	edition, err := directCodingVersionComponent(profile, "rust_edition")
	if err != nil {
		return err
	}
	version, err := directCodingVersionComponent(profile, "rust_version")
	if err != nil {
		return err
	}
	if manifest.Package.Edition != edition || manifest.Package.RustVersion != version {
		return fmt.Errorf("Rust version profile requires edition %s and rust-version %s", edition, version)
	}
	lockVersion, err := directCodingVersionComponent(profile, "cargo_lock")
	if err != nil {
		return err
	}
	var lockfile struct {
		Version int `toml:"version"`
	}
	if err := toml.Unmarshal([]byte(files["Cargo.lock"]), &lockfile); err != nil {
		return err
	}
	if fmt.Sprint(lockfile.Version) != lockVersion {
		return fmt.Errorf("Rust version profile requires Cargo lock format %s", lockVersion)
	}
	return nil
}

func validateJavaVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	release, err := directCodingVersionComponent(profile, "java_release")
	if err != nil {
		return err
	}
	for path := range directCodingAssemblyFiles(assembly) {
		if !strings.HasSuffix(path, ".java") {
			continue
		}
		commands, err := javaCommandLineVerificationCommands(program)
		if err != nil {
			return err
		}
		for _, command := range commands {
			if command.Name == "javac" && len(command.Args) >= 2 &&
				command.Args[0] == "--release" && command.Args[1] == release {
				return nil
			}
		}
		return fmt.Errorf("Java version profile lacks javac --release %s verification", release)
	}
	return fmt.Errorf("Java version profile has no release-%s source", release)
}

func validatePHPVersionProfileAssembly(
	profile directCodingProjectVersionProfile,
	program directCodingProgram,
	assembly directCodingAssembly,
) error {
	files := directCodingAssemblyFiles(assembly)
	compatibility, err := matchPHPVersionProfile(
		profile, map[string]string{"composer.json": files["composer.json"]},
	)
	if err != nil || compatibility != directCodingVersionCompatible {
		return fmt.Errorf("PHP version profile requires its Composer PHP constraint")
	}
	composerImage, err := directCodingVersionComponent(profile, "composer_image")
	if err != nil {
		return err
	}
	nginxImage, err := directCodingVersionComponent(profile, "nginx_image")
	if err != nil {
		return err
	}
	assertion, err := phpRuntimeVersionAssertion(profile)
	if err != nil {
		return err
	}
	for _, required := range []string{composerImage, nginxImage, assertion} {
		if !strings.Contains(files["Dockerfile"]+files["docker-compose.yml"], required) {
			return fmt.Errorf("PHP version profile lacks pinned runtime authority %s", required)
		}
	}
	hasState, err := phpServiceProgramRequiresPostgreSQL(program)
	if err != nil {
		return err
	}
	postgresImage, err := directCodingVersionComponent(profile, "postgres_image")
	if err != nil {
		return err
	}
	if hasState && !strings.Contains(files["docker-compose.yml"], postgresImage) {
		return fmt.Errorf("PHP version profile lacks pinned PostgreSQL runtime authority %s", postgresImage)
	}
	if hasState && files[phpServiceStateVerificationPath] !=
		phpServiceStateVerifierSource("workload:"+program.Workload.SHA256) {
		return fmt.Errorf("PHP version profile state isolation differs from workload authority")
	}
	if !hasState && strings.Contains(files["docker-compose.yml"], postgresImage) {
		return fmt.Errorf("PHP request-local version profile contains an unused PostgreSQL runtime")
	}
	packageManifest, hasManifest := files["package.json"]
	packageLock, hasLock := files["package-lock.json"]
	if hasManifest != hasLock {
		return fmt.Errorf("PHP version profile requires its npm manifest and lock together")
	}
	if !hasManifest {
		return nil
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	nodeImage, err := directCodingVersionComponent(profile, "node_image")
	if err != nil {
		return err
	}
	nodeAssertion, err := nodeExactVersionAssertion(profile)
	if err != nil {
		return err
	}
	if err := validateNPMManifestVersionProfile(
		packageManifest, profile, map[string]string{"node": node},
	); err != nil {
		return err
	}
	if err := validatePinnedNPMLockForProfile(packageManifest, packageLock, profile); err != nil {
		return err
	}
	for _, required := range []string{
		nodeImage, nodeAssertion, "RUN npm ci --ignore-scripts --no-audit --no-fund",
	} {
		if !strings.Contains(files["Dockerfile"], required) {
			return fmt.Errorf("PHP version profile lacks pinned npm runtime authority %s", required)
		}
	}
	return nil
}

func validateNPMManifestVersionProfile(
	source string,
	profile directCodingProjectVersionProfile,
	engines map[string]string,
) error {
	var manifest struct {
		Type            string            `json:"type"`
		Engines         map[string]string `json:"engines"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(source), &manifest); err != nil {
		return err
	}
	if manifest.Type != "module" || !reflect.DeepEqual(manifest.Engines, engines) ||
		!reflect.DeepEqual(manifest.Dependencies, profile.NPMDependencies) ||
		!reflect.DeepEqual(manifest.DevDependencies, profile.NPMDevDependencies) {
		return fmt.Errorf("npm manifest differs from registered version profile %s", profile.ID)
	}
	return nil
}

func directCodingAssemblyFiles(assembly directCodingAssembly) map[string]string {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
	}
	return files
}
