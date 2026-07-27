package omni

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runArchitectCodeContentLane(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	contract := enrichImplementationArchitectContract(buildImplementationArchitectContract(prompt, toolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations), prompt, toolTask, cfg.PrepContext, cfg.SessionMemories)
	contract = enrichImplementationArchitectContractWithProjectMap(contract, cfg.CurrentWorkingDirectory, result.Observations)
	if !hasImplementationArchitectContract(contract) || contract.CurrentItem == nil {
		return false, nil
	}
	if externalAgent, externalAgentName, unavailableReason := selectedAvailableExternalArchitectAgent(cfg); !architectItemIsPackageMetadataUpdate(*contract.CurrentItem) && shouldDelegateArchitectContractToExternalAgent(contract, result.Observations) {
		if externalAgent == nil {
			emitExternalArchitectUnavailable(step, unavailableReason, onEvent)
		} else {
			handled, err := runExternalArchitectAgentLane(ctx, step, prompt, toolTask, contract, cfg, worksiteSurvey, stdout, stderr, onEvent, result, externalAgent, externalAgentName)
			if handled || err != nil {
				return handled, err
			}
		}
	}
	handled := false
	for handledItems := 0; handledItems < len(contract.WorkQueue)+1; handledItems++ {
		contract = enrichImplementationArchitectContract(buildImplementationArchitectContract(prompt, toolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations), prompt, toolTask, cfg.PrepContext, cfg.SessionMemories)
		if !hasImplementationArchitectContract(contract) || contract.CurrentItem == nil {
			break
		}
		item := *contract.CurrentItem
		if architectItemIsPackageMetadataUpdate(item) {
			if _, _, unavailableReason := selectedAvailableExternalArchitectAgent(cfg); shouldDelegateArchitectContractToExternalAgent(contract, result.Observations) && strings.TrimSpace(unavailableReason) != "" {
				emitExternalArchitectUnavailable(step, unavailableReason, onEvent)
			}
			handledPackage, err := runPackageMetadataWorkHandler(step, prompt, contract, cfg, worksiteSurvey, onEvent, result)
			if err != nil {
				return true, err
			}
			if handledPackage {
				handled = true
				if cfg.CodeContentSpecialist == nil {
					if externalAgent, _, _ := selectedAvailableExternalArchitectAgent(cfg); externalAgent == nil {
						return true, nil
					}
				}
				continue
			}
		}
		if externalAgent, externalAgentName, _ := selectedAvailableExternalArchitectAgent(cfg); externalAgent != nil && shouldDelegateArchitectContractToExternalAgent(contract, result.Observations) {
			return runExternalArchitectAgentLane(ctx, step, prompt, toolTask, contract, cfg, worksiteSurvey, stdout, stderr, onEvent, result, externalAgent, externalAgentName)
		}
		if item.Operation == "read" {
			if item.Path == "" || strings.HasSuffix(item.Path, "/") {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "implementation architect read item missing exact file path for " + item.ID,
				})
				return true, nil
			}
			targetPath := filepath.Join(cfg.CurrentWorkingDirectory, item.CWD, item.Path)
			content, err := os.ReadFile(targetPath)
			command := fmt.Sprintf("architect.read %s", filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					emitStructuredCommandEvent(onEvent, "architect_work_item_read_absent", "Implementation architect confirmed target file is absent before create/update", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"item_id": item.ID,
						"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:     step,
						Command:  command,
						ExitCode: 0,
						Stdout:   "(file absent)",
					})
					handled = true
					continue
				}
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					Command:  command,
					ExitCode: 1,
					Stderr:   err.Error(),
				})
				return true, nil
			}
			emitStructuredCommandEvent(onEvent, "architect_work_item_read", "Implementation architect read exact queued file", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
			result.Command = command
			result.ExitCode = 0
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				Command:  command,
				ExitCode: 0,
				Stdout:   string(content),
			})
			handled = true
			continue
		}
		if item.Operation == "delete" {
			if item.Path == "" || strings.HasSuffix(item.Path, "/") {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "implementation architect delete item missing exact file path for " + item.ID,
				})
				return true, nil
			}
			targetPath := filepath.Join(cfg.CurrentWorkingDirectory, item.CWD, item.Path)
			if err := validateArchitectDeleteTarget(cfg.CurrentWorkingDirectory, item, targetPath); err != nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   err.Error(),
				})
				return true, nil
			}
			if err := os.Remove(targetPath); err != nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   err.Error(),
				})
				return true, nil
			}
			command := fmt.Sprintf("architect.delete %s", filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
			emitStructuredCommandEvent(onEvent, "architect_work_item_deleted", "Implementation architect deleted exact queued file after safety validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
			result.Command = command
			result.ExitCode = 0
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				Command:  command,
				ExitCode: 0,
				Stdout:   "architect safety-validated delete " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
			handled = true
			continue
		}
		if item.Operation == "verify" {
			verifyCommand := commandInArchitectCWD(item.CWD, item.Verify)
			if strings.TrimSpace(verifyCommand) == "" {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "implementation architect verify item missing command for " + item.ID,
				})
				return true, nil
			}
			emitStructuredCommandEvent(onEvent, "architect_work_item_verify_started", "Implementation architect started proof command", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"command": truncateStructuredTimelineValue(verifyCommand),
			})
			if err := runStructuredPayloadCommand(ctx, step, verifyCommand, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
				return true, err
			}
			if result.ExitCode == 0 {
				emitStructuredCommandEvent(onEvent, "architect_work_item_verified", "Implementation architect proof command passed", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"item_id": item.ID,
					"command": truncateStructuredTimelineValue(verifyCommand),
				})
			} else {
				emitStructuredCommandEvent(onEvent, "architect_work_item_verify_failed", "Implementation architect proof command failed; returning feedback to architect loop", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"item_id": item.ID,
					"command": truncateStructuredTimelineValue(verifyCommand),
					"stderr":  truncateStructuredTimelineValue(latestFailedCommandOutput(result.Observations, verifyCommand)),
				})
				handled = true
				return handled, nil
			}
			handled = true
			continue
		}
		if item.Operation != "update" && item.Operation != "create" {
			break
		}
		if item.Path == "" || strings.HasSuffix(item.Path, "/") {
			break
		}
		if cfg.CodeContentSpecialist == nil {
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "no capable actor configured for code-owned architect file work item " + item.ID + "; external architect unavailable, local code specialist unavailable, and no deterministic recipe handler matched",
			})
			emitStructuredCommandEvent(onEvent, "architect_work_item_no_capable_actor", "Code-owned architect file work has no available non-shell actor", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"item_id":   item.ID,
				"operation": item.Operation,
				"path":      filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
			return true, nil
		}
		emitStructuredCommandEvent(onEvent, "architect_work_item_started", "Implementation architect started queued code work item", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"item_id":   item.ID,
			"operation": item.Operation,
			"cwd":       item.CWD,
			"path":      item.Path,
		})
		if err := architectImplementationBlockedByMissingTestProbe(contract.WorkQueue, item, cfg.CurrentWorkingDirectory, contract, prompt, result.Observations); err != nil {
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:            step,
				RejectedCommand: fmt.Sprintf("architect.apply %s %s", item.Operation, filepath.ToSlash(filepath.Join(item.CWD, item.Path))),
				ExitCode:        1,
				Stderr:          err.Error(),
			})
			emitStructuredCommandEvent(onEvent, "architect_work_item_test_first_blocked", "Implementation architect blocked implementation until acceptance probe exists", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"reason":  truncateStructuredTimelineValue(err.Error()),
			})
			return true, nil
		}
		targetPath := filepath.Join(cfg.CurrentWorkingDirectory, item.CWD, item.Path)
		existing, _ := os.ReadFile(targetPath)
		proposal, err := generateValidatedCodeContent(ctx, step, prompt, contract, item, string(existing), cfg, worksiteSurvey, onEvent, result)
		if err != nil {
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "code content specialist failed: " + err.Error(),
			})
			return false, fmt.Errorf("code content specialist failed for architect work item %s: %w", item.ID, err)
		}
		if strings.TrimSpace(proposal.Content) == "" {
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "code content specialist returned empty content for architect work item " + item.ID,
			})
			return true, nil
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return true, err
		}
		if err := os.WriteFile(targetPath, []byte(proposal.Content), 0o644); err != nil {
			return true, err
		}
		if _, err := architectWorkItemFileEvidenceValid(item, cfg.CurrentWorkingDirectory, contract, prompt); err != nil {
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:            step,
				RejectedCommand: fmt.Sprintf("architect.apply %s %s", item.Operation, filepath.ToSlash(filepath.Join(item.CWD, item.Path))),
				ExitCode:        1,
				Stderr:          "architect work item evidence rejected after apply: " + err.Error(),
			})
			emitStructuredCommandEvent(onEvent, "architect_work_item_evidence_rejected", "Implementation architect rejected applied file because on-disk evidence failed validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"reason":  truncateStructuredTimelineValue(err.Error()),
			})
			return true, nil
		}
		if architectWorkItemIsTestFirst(item) {
			emitStructuredCommandEvent(onEvent, structuredProofEventAcceptanceProbeCreated, "Implementation architect created or updated a focused acceptance probe", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
		} else {
			emitStructuredCommandEvent(onEvent, structuredProofEventImplementationStarted, "Implementation architect wrote source after validated acceptance probe", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			})
		}
		command := fmt.Sprintf("architect.apply %s %s", item.Operation, filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		contract.ProjectFileMap = markProjectFileMapEntryDone(contract.ProjectFileMap, item.Path)
		emitStructuredCommandEvent(onEvent, "project_file_map_updated", "Project file map marked mapped file complete after validated apply", map[string]string{
			"step":         fmt.Sprintf("%d", step),
			"path":         filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			"revision":     fmt.Sprintf("%d", contract.ProjectFileMap.Revision),
			"open_changes": strings.Join(contract.ProjectFileMap.OpenChanges, ","),
		})
		emitStructuredCommandEvent(onEvent, "architect_work_item_applied", "Implementation architect applied generated code content", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"item_id":   item.ID,
			"path":      filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			"rationale": truncateStructuredTimelineValue(proposal.Rationale),
		})
		result.Command = command
		result.ExitCode = 0
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			Command:  command,
			ExitCode: 0,
			Stdout:   "architect applied " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
		})
		handled = true
	}
	return handled, nil
}

func runPackageMetadataWorkHandler(step int, prompt string, contract ImplementationArchitectContract, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if !hasImplementationArchitectContract(contract) || contract.CurrentItem == nil || !architectItemIsPackageMetadataUpdate(*contract.CurrentItem) {
		return false, nil
	}
	item := *contract.CurrentItem
	targetPath := filepath.Join(cfg.CurrentWorkingDirectory, item.CWD, item.Path)
	if err := validatePackageMetadataDependencyPlan(cfg.CurrentWorkingDirectory, worksiteSurvey); err != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "package metadata dependency plan rejected: " + err.Error(),
		})
		emitStructuredCommandEvent(onEvent, "package_metadata_rejected", "Package metadata handler rejected dependency plan", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"item_id": item.ID,
			"reason":  truncateStructuredTimelineValue(err.Error()),
		})
		return true, nil
	}
	existing, _ := os.ReadFile(targetPath)
	updated, err := deterministicReactPackageMetadata(existing, filepath.Base(filepath.Clean(firstNonEmpty(item.CWD, cfg.CurrentWorkingDirectory, "omnidex-app"))))
	if err != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "package metadata handler failed: " + err.Error(),
		})
		return true, nil
	}
	if err := validateCodeContentProposalForArchitectItem(string(updated), contract, item); err != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "package metadata validation failed: " + err.Error(),
		})
		emitStructuredCommandEvent(onEvent, "package_metadata_rejected", "Package metadata handler output failed validation", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"item_id": item.ID,
			"reason":  truncateStructuredTimelineValue(err.Error()),
		})
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return true, err
	}
	if err := os.WriteFile(targetPath, updated, 0o644); err != nil {
		return true, err
	}
	path := filepath.ToSlash(filepath.Join(item.CWD, item.Path))
	command := fmt.Sprintf("architect.apply %s %s", item.Operation, path)
	summary := packageMetadataReadbackSummary(updated)
	result.Command = command
	result.ExitCode = 0
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:         step,
		Command:      command,
		EvidenceKind: "implementation",
		GeneratedBy:  "package_metadata_handler",
		ExitCode:     0,
		Stdout:       summary,
	})
	emitStructuredCommandEvent(onEvent, "package_metadata_updated", "Deterministic package metadata handler updated package.json", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"item_id": item.ID,
		"path":    path,
	})
	emitStructuredCommandEvent(onEvent, "dependency_metadata_configured", "React/Vite dependency metadata declared by deterministic package handler", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"item_id":  item.ID,
		"packages": strings.Join(reactVitePackageMetadataDependencies(), ","),
	})
	emitStructuredCommandEvent(onEvent, "package_dependencies_declared", "package.json dependency declarations were configured without claiming npm install ran", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"item_id":  item.ID,
		"packages": strings.Join(reactVitePackageMetadataDependencies(), ","),
	})
	emitStructuredCommandEvent(onEvent, "scripts_configured", "Vite package scripts configured by deterministic package handler", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"item_id": item.ID,
		"scripts": "dev,build,preview,test",
	})
	emitStructuredCommandEvent(onEvent, "package_json_valid", "package.json parsed and passed architect metadata validation", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"item_id": item.ID,
		"hash":    packageMetadataSHA256(updated),
	})
	return true, nil
}

func architectItemIsPackageMetadataUpdate(item ArchitectWorkItem) bool {
	if strings.TrimSpace(item.Path) == "" || strings.HasSuffix(strings.TrimSpace(item.Path), "/") {
		return false
	}
	path := filepath.ToSlash(strings.ToLower(strings.TrimSpace(item.Path)))
	if path != "package.json" && !strings.HasSuffix(path, "/package.json") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Operation)) {
	case "create", "update":
		return true
	default:
		return false
	}
}

func validatePackageMetadataDependencyPlan(workingDirectory string, survey WorksiteSurvey) error {
	command := "npm install " + strings.Join(reactVitePackageMetadataDependencies(), " ")
	ledger := []StructuredObjective{{
		ID:          "setup_react_package_metadata",
		Description: "Configure package metadata for a React Vite app",
		Status:      "pending",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
		Packages:    reactVitePackageMetadataDependencies(),
	}}
	return validateStructuredCommandForRunWithSurvey(command, nil, workingDirectory, ledger, survey)
}

func deterministicReactPackageMetadata(existing []byte, fallbackName string) ([]byte, error) {
	pkg := map[string]interface{}{}
	if strings.TrimSpace(string(existing)) != "" {
		if err := json.Unmarshal(existing, &pkg); err != nil {
			return nil, fmt.Errorf("existing package.json is invalid JSON: %w", err)
		}
	}
	name, _ := pkg["name"].(string)
	if strings.TrimSpace(name) == "" {
		pkg["name"] = sanitizePackageMetadataName(fallbackName)
	}
	if _, ok := pkg["version"].(string); !ok {
		pkg["version"] = "0.1.0"
	}
	pkg["type"] = "module"
	scripts := packageStringMap(pkg["scripts"])
	scripts["dev"] = "vite --host 0.0.0.0"
	scripts["build"] = "vite build"
	scripts["preview"] = "vite --host 0.0.0.0"
	scripts["test"] = "node scripts/smoke-test.mjs"
	pkg["scripts"] = scripts
	deps := packageStringMap(pkg["dependencies"])
	for _, dep := range reactVitePackageMetadataDependencies() {
		if strings.TrimSpace(deps[dep]) == "" {
			deps[dep] = "latest"
		}
	}
	pkg["dependencies"] = deps
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	return out, nil
}

func reactVitePackageMetadataDependencies() []string {
	return []string{"react", "react-dom", "vite", "@vitejs/plugin-react"}
}

func packageStringMap(raw interface{}) map[string]string {
	out := map[string]string{}
	values, ok := raw.(map[string]interface{})
	if !ok {
		return out
	}
	for key, value := range values {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func sanitizePackageMetadataName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	clean := strings.Trim(b.String(), "-")
	if clean == "" {
		return "omnidex-app"
	}
	return clean
}

func packageMetadataReadbackSummary(content []byte) string {
	return "package_metadata_updated; package_dependencies_declared=" + strings.Join(reactVitePackageMetadataDependencies(), ",") +
		"; scripts_configured=dev,build,preview,test; package_json_valid=true; sha256=" + packageMetadataSHA256(content) +
		"; snippet=" + truncateStructuredObservation(strings.TrimSpace(string(content)))
}

func packageMetadataSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}
