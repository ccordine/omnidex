package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type structuredDependencyInstallRequest struct {
	Manager  string
	Packages []string
}

func validateStructuredDependencyScope(command string, objectiveLedger []StructuredObjective, workingDirectory string) error {
	requests := structuredDependencyInstallRequests(command)
	if len(requests) == 0 {
		return nil
	}
	allowed := structuredAllowedDependencyPackages(objectiveLedger, workingDirectory)
	blocked := []string{}
	for _, request := range requests {
		for _, pkg := range request.Packages {
			normalized := normalizeDependencyPackageName(pkg)
			if normalized == "" {
				continue
			}
			if !allowed[normalized] {
				blocked = append(blocked, pkg)
			}
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	return fmt.Errorf("dependency scope drift: unrequested package(s) %s; dependencies must be justified by user_explicit, recipe_required, detected_project, or evidence_required_prerequisite objectives", strings.Join(cleanStringList(blocked), ", "))
}

func structuredDependencyInstallRequests(command string) []structuredDependencyInstallRequest {
	requests := []structuredDependencyInstallRequest{}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		switch root {
		case "npm":
			if segment[1] == "install" || segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "npm", Packages: pkgs})
				}
			}
		case "pnpm":
			if segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "pnpm", Packages: pkgs})
				}
			}
		case "yarn":
			if segment[1] == "add" || segment[1] == "install" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "yarn", Packages: pkgs})
				}
			}
		case "go":
			if segment[1] == "get" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "go", Packages: pkgs})
				}
			}
		case "composer":
			if segment[1] == "require" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "composer", Packages: pkgs})
				}
			}
		case "pip", "pip3":
			if segment[1] == "install" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: root, Packages: pkgs})
				}
			}
		case "cargo":
			if segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "cargo", Packages: pkgs})
				}
			}
		}
	}
	return requests
}

func dependencyPackagesFromArgs(args []string) []string {
	packages := []string{}
	skipNext := false
	for _, raw := range args {
		arg := strings.Trim(raw, `"'`)
		if arg == "" {
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if dependencyFlagTakesValue(arg) {
				skipNext = true
			}
			continue
		}
		if strings.Contains(arg, "=") && strings.HasPrefix(arg, "--") {
			continue
		}
		packages = append(packages, arg)
	}
	return cleanStringList(packages)
}

func dependencyFlagTakesValue(flag string) bool {
	switch flag {
	case "-r", "--requirement", "-c", "--constraint", "--index-url", "--extra-index-url", "--registry", "--prefix", "--global-folder", "--modules-folder":
		return true
	default:
		return false
	}
}

func structuredAllowedDependencyPackages(objectiveLedger []StructuredObjective, workingDirectory string) map[string]bool {
	allowed := map[string]bool{}
	for _, objective := range objectiveLedger {
		objective, ok := normalizeStructuredObjective(objective)
		if !ok || !structuredObjectiveSourceCanExecute(objective.Source) {
			continue
		}
		for _, pkg := range objective.Packages {
			if normalized := normalizeDependencyPackageName(pkg); normalized != "" {
				allowed[normalized] = true
			}
		}
		for _, pkg := range inferredDependencyPackagesForObjective(objective) {
			allowed[pkg] = true
		}
	}
	for _, pkg := range detectedProjectDependencyPackages(workingDirectory) {
		allowed[pkg] = true
	}
	return allowed
}

func structuredObjectiveSourceCanExecute(source string) bool {
	switch normalizeStructuredObjectiveSource(source) {
	case structuredObjectiveSourceUserExplicit, structuredObjectiveSourceRecipeRequired, structuredObjectiveSourceDetectedProject, structuredObjectiveSourceEvidenceRequiredPrerequisite:
		return true
	default:
		return false
	}
}

func inferredDependencyPackagesForObjective(objective StructuredObjective) []string {
	text := normalizedDependencyText(objective.ID + " " + objective.Description)
	out := []string{}
	if strings.Contains(text, " react ") {
		out = append(out, "react", "react-dom", "vite", "@vitejs/plugin-react")
	}
	if strings.Contains(text, " tailwind ") || strings.Contains(text, " tailwindcss ") {
		out = append(out, "tailwindcss", "postcss", "autoprefixer", "@tailwindcss/vite")
	}
	if strings.Contains(text, " typescript ") {
		out = append(out, "typescript", "@types/react", "@types/react-dom")
	}
	if (strings.Contains(text, " chess ") || strings.Contains(text, " legal move ") || strings.Contains(text, " legal moves ") || strings.Contains(text, " rules library ")) &&
		(strings.Contains(text, " rust ") || strings.Contains(text, " cargo ")) {
		out = append(out, "chess", "shakmaty")
	}
	return out
}

func detectedProjectDependencyPackages(workingDirectory string) []string {
	if strings.TrimSpace(workingDirectory) == "" {
		return nil
	}
	blob, err := os.ReadFile(filepath.Join(workingDirectory, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies         map[string]interface{} `json:"dependencies"`
		DevDependencies      map[string]interface{} `json:"devDependencies"`
		PeerDependencies     map[string]interface{} `json:"peerDependencies"`
		OptionalDependencies map[string]interface{} `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(blob, &pkg); err != nil {
		return nil
	}
	out := []string{}
	for _, deps := range []map[string]interface{}{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies, pkg.OptionalDependencies} {
		for dep := range deps {
			if normalized := normalizeDependencyPackageName(dep); normalized != "" {
				out = append(out, normalized)
			}
		}
	}
	return cleanStringList(out)
}

func normalizeDependencyPackageName(pkg string) string {
	clean := strings.Trim(strings.TrimSpace(pkg), `"'`)
	if clean == "" {
		return ""
	}
	if strings.HasPrefix(clean, "git+") || strings.Contains(clean, "://") || strings.HasPrefix(clean, ".") || strings.HasPrefix(clean, "/") {
		return ""
	}
	if at := strings.LastIndex(clean, "@"); at > 0 {
		clean = clean[:at]
	}
	return strings.ToLower(clean)
}

func normalizedDependencyText(text string) string {
	var b strings.Builder
	b.WriteByte(' ')
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteByte(' ')
	return b.String()
}

func validateStructuredCommandWorkspaceProtection(command, workingDirectory string) error {
	workspace := strings.TrimSpace(workingDirectory)
	if workspace == "" {
		return nil
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return nil
	}
	workspaceAbs = filepath.Clean(workspaceAbs)
	segments := structuredCommandSegments(command)
	deletedTargets := map[string]struct{}{}
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		switch root {
		case "rm", "rmdir":
			for _, target := range structuredCommandPathArgs(segment[1:], workspaceAbs) {
				if pathIsSameOrAncestor(target, workspaceAbs) {
					return fmt.Errorf("command attempts to remove the active working directory or one of its parents; creation/build tasks must preserve existing directories")
				}
				deletedTargets[target] = struct{}{}
			}
		case "mv":
			args := structuredCommandPathArgs(segment[1:], workspaceAbs)
			if len(args) > 0 && pathIsSameOrAncestor(args[0], workspaceAbs) {
				return fmt.Errorf("command attempts to move the active working directory or one of its parents; creation/build tasks must preserve existing directories")
			}
		case "mkdir":
			for _, target := range structuredCommandPathArgs(segment[1:], workspaceAbs) {
				if _, deleted := deletedTargets[target]; deleted {
					return fmt.Errorf("command deletes and recreates the same path; use mkdir -p or update files in place instead")
				}
			}
		}
	}
	return nil
}

func structuredCommandSegments(command string) [][]string {
	fields := strings.Fields(command)
	segments := [][]string{}
	current := []string{}
	for _, field := range fields {
		token := cleanCommandPathToken(field)
		switch token {
		case "&&", "||", ";", "|":
			if len(current) > 0 {
				segments = append(segments, current)
				current = []string{}
			}
		default:
			current = append(current, field)
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

func structuredCommandPathArgs(args []string, workspaceAbs string) []string {
	targets := []string{}
	stopOptions := false
	for _, arg := range args {
		candidate := cleanCommandPathToken(arg)
		if candidate == "" {
			continue
		}
		if candidate == "--" {
			stopOptions = true
			continue
		}
		if !stopOptions && strings.HasPrefix(candidate, "-") {
			continue
		}
		if strings.Contains(candidate, "=") || isShellRedirectToken(candidate) {
			continue
		}
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			continue
		}
		resolved, pathLike := resolveCommandPathToken(candidate, workspaceAbs)
		if !pathLike {
			continue
		}
		targets = append(targets, filepath.Clean(resolved))
	}
	return targets
}

func pathIsSameOrAncestor(candidate, target string) bool {
	candidate = filepath.Clean(candidate)
	target = filepath.Clean(target)
	if candidate == target {
		return true
	}
	rel, err := filepath.Rel(candidate, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))
}

func repeatedFailedStructuredCommand(command string, observations []StructuredCommandObservation) bool {
	return false
}

func repeatedSuccessfulStructuredCommand(command string, observations []StructuredCommandObservation) bool {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return false
	}
	if structuredCommandIsVerifierRerunCandidate(command) {
		return false
	}
	for i, obs := range observations {
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			if structuredObservationsContainMutationAfter(observations, i) {
				return false
			}
			return true
		}
	}
	return false
}

func structuredCommandIsVerifierRerunCandidate(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "npm run build") ||
		strings.Contains(lower, "npm test") ||
		strings.Contains(lower, "go test") ||
		strings.Contains(lower, "cargo test") ||
		strings.Contains(lower, "cargo build") ||
		strings.Contains(lower, "zig build") ||
		strings.Contains(lower, "pytest") ||
		strings.Contains(lower, "pnpm test") ||
		strings.Contains(lower, "yarn test")
}

func structuredObservationsContainMutationAfter(observations []StructuredCommandObservation, index int) bool {
	for _, obs := range observations[index+1:] {
		if structuredObservationMutatedWorkspace(obs) {
			return true
		}
	}
	return false
}

func structuredObservationMutatedWorkspace(obs StructuredCommandObservation) bool {
	if obs.ExitCode != 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(obs.EvidenceKind), "implementation") {
		return true
	}
	if strings.TrimSpace(obs.GeneratedBy) == "package_metadata_handler" {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(obs.Command))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "architect.apply") ||
		strings.Contains(lower, "npm install") ||
		strings.Contains(lower, "pnpm install") ||
		strings.Contains(lower, "yarn add") ||
		strings.Contains(lower, "go mod tidy") ||
		strings.Contains(lower, "cargo add") ||
		strings.Contains(lower, "cat >") ||
		strings.Contains(lower, "tee ") ||
		strings.Contains(lower, "apply_patch") ||
		strings.Contains(lower, " > ")
}

func previousSuccessfulStructuredCommandObservation(command string, observations []StructuredCommandObservation) (StructuredCommandObservation, bool) {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return StructuredCommandObservation{}, false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			return obs, true
		}
	}
	return StructuredCommandObservation{}, false
}

func repeatedRejectedCommandCount(command string, observations []StructuredCommandObservation) int {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return 0
	}
	count := 0
	for _, obs := range observations {
		if normalizeStructuredCommandForComparison(obs.RejectedCommand) == normalized {
			count++
		}
	}
	return count
}

func normalizeStructuredCommandForComparison(command string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
}
