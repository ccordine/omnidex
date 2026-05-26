package omni

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type scaffoldSpec struct {
	Command    string
	Name       string
	Kind       string
	TrailingCD string
}

func scaffoldCommandSpec(command, workingDir string) (scaffoldSpec, bool) {
	_ = workingDir
	parts := splitShellAndChain(command)
	if len(parts) == 0 {
		return scaffoldSpec{}, false
	}
	first := strings.TrimSpace(parts[0])
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return scaffoldSpec{}, false
	}
	fields = stripCommandEnvironmentPrefix(fields)
	if len(fields) == 0 {
		return scaffoldSpec{}, false
	}
	spec := scaffoldSpec{Command: first}
	if len(parts) > 1 {
		if cd := trailingCDPart(parts[1:]); cd != "" {
			spec.TrailingCD = cd
		}
	}
	if len(fields) >= 3 && cleanCommandPathToken(fields[0]) == "npx" && fields[1] == "create-react-app" {
		spec.Kind = "create-react-app"
		spec.Name = cleanProjectNameToken(fields[2])
		return spec, spec.Name != ""
	}
	if len(fields) >= 5 && cleanCommandPathToken(fields[0]) == "npm" && fields[1] == "create" && strings.HasPrefix(fields[2], "vite") {
		spec.Kind = "vite"
		spec.Name = firstNonFlagProjectName(fields[3:])
		return spec, spec.Name != ""
	}
	if len(fields) >= 5 && cleanCommandPathToken(fields[0]) == "npm" && fields[1] == "init" && strings.HasPrefix(fields[2], "vite") {
		spec.Kind = "vite"
		spec.Name = firstNonFlagProjectName(fields[3:])
		return spec, spec.Name != ""
	}
	return scaffoldSpec{}, false
}

func splitShellAndChain(command string) []string {
	parts := strings.Split(command, "&&")
	out := []string{}
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func trailingCDPart(parts []string) string {
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 2 && fields[0] == "cd" {
			return cleanProjectNameToken(fields[1])
		}
	}
	return ""
}

func firstNonFlagProjectName(fields []string) string {
	for _, field := range fields {
		clean := cleanProjectNameToken(field)
		if clean == "" || strings.HasPrefix(clean, "-") || clean == "--" {
			continue
		}
		return clean
	}
	return ""
}

func cleanProjectNameToken(token string) string {
	token = strings.Trim(strings.TrimSpace(token), `"'`)
	if token == "." || token == "./" {
		return ""
	}
	return filepath.Clean(token)
}

func applyScaffoldTargetRootPromotion(step int, command, workingDirectory string, stdoutBuf *bytes.Buffer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) {
	spec, ok := scaffoldCommandSpec(command, workingDirectory)
	if !ok || result == nil {
		return
	}
	root := filepath.Join(structuredPromptWorkingDirectory(workingDirectory), spec.Name)
	if !dirExists(root) {
		return
	}
	pkgPath := filepath.Join(root, "package.json")
	srcPath := filepath.Join(root, "src")
	pkgExists := fileExists(pkgPath)
	srcExists := dirExists(srcPath)
	emitStructuredCommandEvent(onEvent, "scaffold_project_detected", "Scaffold command created nested project directory", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"name":       spec.Name,
		"kind":       spec.Kind,
		"target_dir": root,
	})
	if spec.TrailingCD != "" {
		emitStructuredCommandEvent(onEvent, "scaffold_trailing_cd_ignored", "Trailing cd in scaffold command treated as advisory; runtime target root controls future cwd", map[string]string{
			"step":        fmt.Sprintf("%d", step),
			"trailing_cd": spec.TrailingCD,
		})
	}
	if !pkgExists {
		return
	}
	result.TargetRoot = root
	_ = refreshWorkspaceArtifactsAfterScaffold(root)
	routeFiles := scaffoldRouteFiles(root)
	if stdoutBuf != nil {
		stdoutBuf.WriteString("\nscaffold_project_detected: " + spec.Name)
		stdoutBuf.WriteString("\nnew_target_root: " + root)
		stdoutBuf.WriteString("\nproject_directory_exists: true")
		stdoutBuf.WriteString("\npackage_json_exists: true")
		stdoutBuf.WriteString(fmt.Sprintf("\nsrc_directory_exists: %t", srcExists))
		for _, file := range routeFiles {
			stdoutBuf.WriteString("\nroute_file: " + file)
		}
	}
	emitStructuredCommandEvent(onEvent, "target_root_promoted_after_scaffold", "Runtime promoted scaffold-created directory as active target root", map[string]string{
		"step":          fmt.Sprintf("%d", step),
		"name":          spec.Name,
		"target_root":   root,
		"package_json":  fmt.Sprintf("%t", pkgExists),
		"src_directory": fmt.Sprintf("%t", srcExists),
	})
	emitStructuredCommandEvent(onEvent, "workspace_route_refreshed_after_mutation", "Workspace route refreshed after scaffold target-root promotion", map[string]string{
		"step":        fmt.Sprintf("%d", step),
		"target_root": root,
		"route_files": strings.Join(routeFiles, ","),
	})
}

func refreshWorkspaceArtifactsAfterScaffold(root string) error {
	indexPath := filepath.Join(root, ".omni", "index.json")
	index, err := UpdateWorkspaceIndex(root, indexPath, 0)
	if err != nil {
		return err
	}
	if err := WriteWorkspaceIndex(index, indexPath); err != nil {
		return err
	}
	cm, err := UpdateCodebaseMap(root, DefaultCodebaseMapPath(root), CodebaseMapConfig{})
	if err != nil {
		return err
	}
	return WriteCodebaseMap(cm, DefaultCodebaseMapPath(root))
}

func scaffoldRouteFiles(root string) []string {
	candidates := []string{"package.json", "src/index.js", "src/main.jsx", "src/main.tsx", "src/App.js", "src/App.jsx", "src/App.tsx"}
	out := []string{}
	for _, rel := range candidates {
		if fileExists(filepath.Join(root, rel)) {
			out = append(out, rel)
		}
	}
	if len(out) > 0 {
		return out
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			if entry != nil && entry.IsDir() {
				switch entry.Name() {
				case "node_modules", ".git", "build", "dist":
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		if len(out) >= 12 {
			return filepath.SkipAll
		}
		return nil
	})
	return cleanStringList(out)
}

func scaffoldObservationSatisfiesObjective(command, output, target, cwd string) bool {
	spec, ok := scaffoldCommandSpec(command, cwd)
	if !ok {
		return false
	}
	root := filepath.Join(structuredPromptWorkingDirectory(cwd), spec.Name)
	if strings.Contains(output, "new_target_root:") {
		root = scaffoldTargetRootFromOutput(output, root)
	}
	switch {
	case strings.Contains(target, "initialize") || strings.Contains(target, "scaffold") || strings.Contains(target, "project"):
		return dirExists(root) && fileExists(filepath.Join(root, "package.json"))
	case strings.Contains(target, "install") || strings.Contains(target, "dependencies"):
		return fileExists(filepath.Join(root, "package.json")) && (dirExists(filepath.Join(root, "node_modules")) || fileExists(filepath.Join(root, "package-lock.json")) || fileExists(filepath.Join(root, "yarn.lock")) || fileExists(filepath.Join(root, "pnpm-lock.yaml")))
	case strings.Contains(target, "entrypoint") || strings.Contains(target, "entry point"):
		return entrypointExists(root)
	case strings.Contains(target, "initial app") || strings.Contains(target, "setup_initial_app") || strings.Contains(target, "app.js") || strings.Contains(target, "app.jsx"):
		return fileExists(filepath.Join(root, "src", "App.js")) || fileExists(filepath.Join(root, "src", "App.jsx")) || fileExists(filepath.Join(root, "src", "App.tsx"))
	default:
		return false
	}
}

func scaffoldTargetRootFromOutput(output, fallback string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "new_target_root:") {
			if value := strings.TrimSpace(strings.TrimPrefix(line, "new_target_root:")); value != "" {
				return value
			}
		}
	}
	return fallback
}

func entrypointExists(root string) bool {
	for _, rel := range []string{"src/index.js", "src/index.jsx", "src/main.jsx", "src/main.tsx"} {
		if fileExists(filepath.Join(root, rel)) {
			return true
		}
	}
	return false
}
