package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

func hasImplementationArchitectContract(contract ImplementationArchitectContract) bool {
	return strings.TrimSpace(contract.TargetRoot) != "" || len(contract.EditSurface) > 0 || len(contract.ProofCommands) > 0
}

func validateCommandAgainstImplementationArchitectContract(command string, contract ImplementationArchitectContract) error {
	if !hasImplementationArchitectContract(contract) || strings.TrimSpace(command) == "" {
		return nil
	}
	target := strings.TrimSpace(contract.TargetRoot)
	if target == "" || target == "." {
		return validateCommandAgainstArchitectCurrentItem(command, contract)
	}
	if structuredCommandLooksDependencyInstall(command) || structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	target = filepath.ToSlash(strings.ToLower(strings.Trim(target, "/")))
	if commandChangesIntoProjectRoot(cmd, target) || strings.Contains(cmd, target+"/") {
		return validateCommandAgainstArchitectCurrentItem(command, contract)
	}
	return errArchitectContract("command must target architect root %q by cd-ing into it or using paths under it", contract.TargetRoot)
}

func validateCommandAgainstArchitectCurrentItem(command string, contract ImplementationArchitectContract) error {
	if contract.CurrentItem == nil {
		return nil
	}
	item := *contract.CurrentItem
	if item.Operation == "verify" {
		expected := normalizeStructuredCommandForComparison(commandInArchitectCWD(item.CWD, item.Verify))
		if normalizeStructuredCommandForComparison(command) == expected {
			return nil
		}
		return errArchitectContract("current work item %q requires verification command %q", item.ID, commandInArchitectCWD(item.CWD, item.Verify))
	}
	if architectItemIsPackageMetadataUpdate(item) && structuredCommandLooksPackageMetadataOperation(command) {
		return nil
	}
	if item.Path == "" {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	path := filepath.ToSlash(strings.ToLower(item.Path))
	if item.CWD != "" && item.CWD != "." {
		if commandChangesIntoProjectRoot(cmd, strings.ToLower(item.CWD)) {
			if strings.Contains(cmd, path) {
				return nil
			}
		}
		full := filepath.ToSlash(strings.ToLower(filepath.Join(item.CWD, item.Path)))
		if strings.Contains(cmd, full) {
			return nil
		}
		return errArchitectContract("current work item %q requires path %q under cwd %q", item.ID, item.Path, item.CWD)
	}
	if strings.Contains(cmd, path) {
		return nil
	}
	return errArchitectContract("current work item %q requires path %q", item.ID, item.Path)
}

func errArchitectContract(format string, args ...interface{}) error {
	return &implementationArchitectValidationError{message: formatArchitectError(format, args...)}
}

type implementationArchitectValidationError struct {
	message string
}

func (e *implementationArchitectValidationError) Error() string {
	return e.message
}

func formatArchitectError(format string, args ...interface{}) string {
	return "architect contract violation: " + fmt.Sprintf(format, args...)
}
