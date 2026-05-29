package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProjectFileMapStatus string

const (
	ProjectFileMapStatusPlanned    ProjectFileMapStatus = "planned"
	ProjectFileMapStatusInProgress ProjectFileMapStatus = "in_progress"
	ProjectFileMapStatusDone       ProjectFileMapStatus = "done"
	ProjectFileMapStatusRemoved    ProjectFileMapStatus = "removed"
)

type ProjectFileMapEntry struct {
	Path       string               `json:"path"`
	CWD        string               `json:"cwd,omitempty"`
	Role       string               `json:"role,omitempty"`
	Purpose    string               `json:"purpose,omitempty"`
	Status     ProjectFileMapStatus `json:"status"`
	Operation  string               `json:"operation,omitempty"`
	DependsOn  []string             `json:"depends_on,omitempty"`
	UsedBy     []string             `json:"used_by,omitempty"`
	WorkItemID string               `json:"work_item_id,omitempty"`
	Verify     string               `json:"verify,omitempty"`
}

type ProjectFileMap struct {
	TargetRoot   string                `json:"target_root"`
	Revision     int                   `json:"revision"`
	SourcePrompt string                `json:"source_prompt,omitempty"`
	Files        []ProjectFileMapEntry `json:"files"`
	ActiveFile   *ProjectFileMapEntry  `json:"active_file,omitempty"`
	OpenChanges  []string              `json:"open_changes,omitempty"`
	TreeSummary  string                `json:"tree_summary,omitempty"`
}

func buildProjectFileMapFromContract(contract ImplementationArchitectContract) ProjectFileMap {
	if !hasImplementationArchitectContract(contract) {
		return ProjectFileMap{}
	}
	entries := make([]ProjectFileMapEntry, 0, len(contract.WorkQueue))
	for _, item := range contract.WorkQueue {
		if item.Operation == "verify" || item.Path == "" || strings.HasSuffix(item.Path, "/") {
			continue
		}
		if strings.HasPrefix(item.ID, "read_before_") {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(item.Path))
		entries = append(entries, ProjectFileMapEntry{
			Path:       path,
			CWD:        filepath.ToSlash(strings.TrimSpace(item.CWD)),
			Role:       projectFileRoleForPath(path, item),
			Purpose:    strings.TrimSpace(item.Description),
			Status:     ProjectFileMapStatusPlanned,
			Operation:  item.Operation,
			DependsOn:  projectFileDefaultDependencies(path),
			WorkItemID: item.ID,
			Verify:     item.Verify,
		})
	}
	entries = linkProjectFileUsedBy(entries)
	return ProjectFileMap{
		TargetRoot:   contract.TargetRoot,
		Revision:     1,
		SourcePrompt: contract.SourcePrompt,
		Files:        entries,
		TreeSummary:  renderProjectFileTreeSummary(entries),
	}
}

func projectFileRoleForPath(path string, item ArchitectWorkItem) string {
	lower := strings.ToLower(path)
	switch {
	case lower == "package.json":
		return "package_manifest"
	case strings.Contains(lower, "vite.config"):
		return "build_config"
	case lower == "index.html":
		return "html_shell"
	case strings.Contains(lower, "main.jsx") || strings.Contains(lower, "main.js") || strings.Contains(lower, "index.js"):
		return "app_mount"
	case strings.Contains(lower, "smoke-test"):
		return "acceptance_probe"
	case strings.HasSuffix(lower, ".css"):
		return "stylesheet"
	case strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx"):
		return "application_source"
	default:
		return strings.TrimSpace(item.Operation) + "_file"
	}
}

func projectFileDefaultDependencies(path string) []string {
	lower := strings.ToLower(filepath.ToSlash(path))
	switch lower {
	case "src/app.js", "src/app.jsx", "src/App.js", "src/App.jsx":
		return []string{"scripts/smoke-test.mjs", "src/main.jsx", "index.html", "package.json"}
	case "src/app.css", "src/App.css":
		return []string{"src/App.js", "scripts/smoke-test.mjs"}
	case "src/main.jsx", "src/main.js", "src/index.js":
		return []string{"index.html", "src/App.js"}
	case "index.html":
		return []string{"package.json", "vite.config.js"}
	case "scripts/smoke-test.mjs":
		return []string{"package.json"}
	case "vite.config.js":
		return []string{"package.json"}
	default:
		return nil
	}
}

func linkProjectFileUsedBy(entries []ProjectFileMapEntry) []ProjectFileMapEntry {
	index := map[string]int{}
	for i, entry := range entries {
		index[strings.ToLower(entry.Path)] = i
	}
	for _, entry := range entries {
		for _, dep := range entry.DependsOn {
			if j, ok := index[strings.ToLower(dep)]; ok {
				usedBy := entries[j].UsedBy
				found := false
				for _, existing := range usedBy {
					if strings.EqualFold(existing, entry.Path) {
						found = true
						break
					}
				}
				if !found {
					entries[j].UsedBy = append(entries[j].UsedBy, entry.Path)
				}
			}
		}
	}
	return entries
}

func syncProjectFileMapFromDisk(projectMap ProjectFileMap, workingDir string, contract ImplementationArchitectContract, prompt string, observations []StructuredCommandObservation) ProjectFileMap {
	if len(projectMap.Files) == 0 {
		return projectMap
	}
	targetRoot := strings.TrimSpace(projectMap.TargetRoot)
	if targetRoot == "" {
		targetRoot = "."
	}
	for i, entry := range projectMap.Files {
		if entry.Status == ProjectFileMapStatusRemoved {
			continue
		}
		item := ArchitectWorkItem{
			ID:        entry.WorkItemID,
			Operation: entry.Operation,
			CWD:       entry.CWD,
			Path:      entry.Path,
		}
		if entry.Operation == "" {
			item.Operation = "update"
		}
		if architectWorkItemSatisfied(item, workingDir, contract, observations) {
			projectMap.Files[i].Status = ProjectFileMapStatusDone
			continue
		}
		if _, err := architectWorkItemFileEvidenceValid(item, workingDir, contract, prompt); err == nil {
			projectMap.Files[i].Status = ProjectFileMapStatusDone
			continue
		}
		targetPath := filepath.Join(workingDir, entry.CWD, entry.Path)
		if _, err := os.Stat(targetPath); err != nil {
			projectMap.Files[i].Status = ProjectFileMapStatusPlanned
			continue
		}
		if entry.Status != ProjectFileMapStatusInProgress {
			projectMap.Files[i].Status = ProjectFileMapStatusPlanned
		}
	}
	projectMap.OpenChanges = projectFileMapOpenChanges(projectMap)
	if active := projectFileMapCurrentActive(projectMap); active != nil {
		projectMap.ActiveFile = active
		for i := range projectMap.Files {
			if strings.EqualFold(projectMap.Files[i].Path, active.Path) {
				projectMap.Files[i].Status = ProjectFileMapStatusInProgress
			}
		}
	} else {
		projectMap.ActiveFile = nil
	}
	projectMap.TreeSummary = renderProjectFileTreeSummary(projectMap.Files)
	return projectMap
}

func projectFileMapOpenChanges(projectMap ProjectFileMap) []string {
	out := []string{}
	for _, entry := range projectMap.Files {
		switch entry.Status {
		case ProjectFileMapStatusPlanned, ProjectFileMapStatusInProgress:
			out = append(out, entry.Path+"("+string(entry.Status)+")")
		}
	}
	return out
}

func projectFileMapCurrentActive(projectMap ProjectFileMap) *ProjectFileMapEntry {
	for _, entry := range projectMap.Files {
		if entry.Status == ProjectFileMapStatusDone || entry.Status == ProjectFileMapStatusRemoved {
			continue
		}
		if !projectFileMapDependenciesDone(projectMap, entry) {
			continue
		}
		copy := entry
		return &copy
	}
	return nil
}

func projectFileMapDependenciesDone(projectMap ProjectFileMap, entry ProjectFileMapEntry) bool {
	for _, dep := range entry.DependsOn {
		if projectFileMapEntryStatus(projectMap, dep) != ProjectFileMapStatusDone {
			return false
		}
	}
	return true
}

func projectFileMapEntryStatus(projectMap ProjectFileMap, path string) ProjectFileMapStatus {
	for _, entry := range projectMap.Files {
		if strings.EqualFold(entry.Path, path) {
			return entry.Status
		}
	}
	return ProjectFileMapStatusPlanned
}

func projectFileMapHasPendingFiles(projectMap ProjectFileMap) bool {
	return projectFileMapCurrentActive(projectMap) != nil
}

func renderProjectFileTreeSummary(entries []ProjectFileMapEntry) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		status := string(entry.Status)
		if status == "" {
			status = string(ProjectFileMapStatusPlanned)
		}
		line := fmt.Sprintf("%s [%s] %s", entry.Path, status, strings.TrimSpace(entry.Purpose))
		if len(entry.DependsOn) > 0 {
			line += " deps=" + strings.Join(entry.DependsOn, ",")
		}
		if len(entry.UsedBy) > 0 {
			line += " used_by=" + strings.Join(entry.UsedBy, ",")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func enrichImplementationArchitectContractWithProjectMap(contract ImplementationArchitectContract, workingDir string, observations []StructuredCommandObservation) ImplementationArchitectContract {
	if !hasImplementationArchitectContract(contract) {
		return contract
	}
	projectMap := buildProjectFileMapFromContract(contract)
	projectMap = syncProjectFileMapFromDisk(projectMap, workingDir, contract, architectContractPrompt(contract), observations)
	contract.ProjectFileMap = projectMap
	if projectMap.ActiveFile != nil {
		for i := range contract.WorkQueue {
			if contract.WorkQueue[i].ID == projectMap.ActiveFile.WorkItemID {
				contract.CurrentItem = &contract.WorkQueue[i]
				break
			}
		}
	}
	return contract
}

func markProjectFileMapEntryDone(projectMap ProjectFileMap, path string) ProjectFileMap {
	path = filepath.ToSlash(strings.TrimSpace(path))
	for i, entry := range projectMap.Files {
		if strings.EqualFold(entry.Path, path) {
			projectMap.Files[i].Status = ProjectFileMapStatusDone
			projectMap.Revision++
			break
		}
	}
	projectMap.OpenChanges = projectFileMapOpenChanges(projectMap)
	projectMap.ActiveFile = projectFileMapCurrentActive(projectMap)
	projectMap.TreeSummary = renderProjectFileTreeSummary(projectMap.Files)
	return projectMap
}

func validateCommandAgainstProjectFileMap(command, workingDir string, projectMap ProjectFileMap) error {
	if len(projectMap.Files) == 0 || strings.TrimSpace(command) == "" {
		return nil
	}
	if !structuredCommandLooksMutating(command) || structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(command)), "architect.") {
		return nil
	}
	targets := mutationWriteTargetPaths(command)
	if len(targets) == 0 {
		if touchTargetsProjectSourceArtifact(command) {
			return fmt.Errorf("project_file_map: touch is forbidden for mapped implementation; the active mapped file must be written by the code specialist or architect.apply")
		}
		return nil
	}
	for _, target := range targets {
		if !projectFileMapContainsPath(projectMap, target) {
			return fmt.Errorf("project_file_map: mutation target %q is not in the active project file map; update the map before creating or editing unlisted files", filepath.ToSlash(target))
		}
	}
	if active := projectMap.ActiveFile; active != nil && len(targets) == 1 {
		if !strings.EqualFold(filepath.ToSlash(targets[0]), active.Path) {
			return fmt.Errorf("project_file_map: active mapped file is %q; do not mutate %q until the active file is complete", active.Path, filepath.ToSlash(targets[0]))
		}
	}
	return validateConflictingEntrypointMutation(command, workingDir)
}

func projectFileMapContainsPath(projectMap ProjectFileMap, path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	for _, entry := range projectMap.Files {
		if entry.Status == ProjectFileMapStatusRemoved {
			continue
		}
		if strings.EqualFold(entry.Path, path) {
			return true
		}
	}
	return false
}

func activeProjectFileMapFromResult(prompt, toolTask, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) ProjectFileMap {
	contract := enrichImplementationArchitectContractWithProjectMap(
		buildImplementationArchitectContract(prompt, toolTask, workingDir, survey, observations),
		workingDir,
		observations,
	)
	return contract.ProjectFileMap
}

func projectFileMapPolicyLines() []string {
	return []string{
		"The project_file_map is the authoritative planned file tree for the active implementation.",
		"Do not create, touch, or mutate source files that are not listed in project_file_map.files unless you first update the map through architect/planner scope changes.",
		"Work one mapped file at a time using project_file_map.active_file; specialists must integrate with depends_on and used_by links.",
		"Shell commands must not use touch for mapped source files; write substantive content through code specialist output applied by architect.apply.",
		"Completion requires every mapped file to reach status done with validated on-disk evidence.",
	}
}

func integrationContextForProjectFileEntry(projectMap ProjectFileMap, path string) string {
	entry := projectFileMapEntryByPath(projectMap, path)
	if entry == nil {
		return ""
	}
	parts := []string{
		"Active mapped file: " + entry.Path,
		"Role: " + entry.Role,
	}
	if strings.TrimSpace(entry.Purpose) != "" {
		parts = append(parts, "Purpose: "+entry.Purpose)
	}
	if len(entry.DependsOn) > 0 {
		parts = append(parts, "Must integrate with: "+strings.Join(entry.DependsOn, ", "))
	}
	if len(entry.UsedBy) > 0 {
		parts = append(parts, "Used by: "+strings.Join(entry.UsedBy, ", "))
	}
	return strings.Join(parts, "\n")
}

func projectFileMapEntryByPath(projectMap ProjectFileMap, path string) *ProjectFileMapEntry {
	for _, entry := range projectMap.Files {
		if strings.EqualFold(entry.Path, path) {
			copy := entry
			return &copy
		}
	}
	return nil
}
