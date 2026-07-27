package worker

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const implementationControlCommand = "CONTROL_PLANE_COMMAND: Return exactly one raw JSON object matching the contract above. Begin with { and end with }. Do not use Markdown fences or add prose."

type implementationFileDecision struct {
	RoleID     string `json:"role_id"`
	WorkItemID string `json:"work_item_id"`
	Status     string `json:"status"`
	Path       string `json:"path"`
	Content    string `json:"content"`
	Error      string `json:"error"`
}

type implementationReviewDecision struct {
	RoleID     string   `json:"role_id"`
	WorkItemID string   `json:"work_item_id"`
	Verdict    string   `json:"verdict"`
	Findings   []string `json:"findings"`
	Authority  string   `json:"-"`
}

type implementationTriageDecision struct {
	RoleID             string `json:"role_id"`
	VerificationItemID string `json:"verification_item_id"`
	OwnerID            string `json:"owner_id"`
	Feedback           string `json:"feedback"`
}

var implementationPlaceholderPattern = regexp.MustCompile(`(?im)(\bTODO\b|\bFIXME\b|\bplaceholder\b|\bstub\b|\bfuture\s+(?:logic|work|implementation)\b|IMPLEMENT[ _-]?ME|not implemented|unimplemented!\s*\(|panic\s*\(\s*["']not implemented)`)
var implementationModulePlaceholderPattern = regexp.MustCompile(`(?i)(your[-_ ]module|example\.com/(?:your|my)[-_/]|github\.com/(?:your(?:[-_ ]?(?:user|username|name))?|username|user)/|<module(?:[-_ ]path)?>)`)

func parseImplementationFileDecision(raw string, item artifacts.ImplementationWorkItem) (implementationFileDecision, error) {
	var decision implementationFileDecision
	if err := decodeStrictImplementationJSON(raw, &decision); err != nil {
		return decision, fmt.Errorf("decode file worker decision: %w", err)
	}
	violations := make([]string, 0, 6)
	if decision.RoleID != "file_worker" {
		violations = append(violations, fmt.Sprintf("file worker role drift: received %q", decision.RoleID))
	}
	if decision.WorkItemID != item.ID {
		violations = append(violations, fmt.Sprintf("file worker work item drift: expected %q, received %q", item.ID, decision.WorkItemID))
	}
	if decision.Path != item.Path {
		violations = append(violations, fmt.Sprintf("file worker path drift: expected %q, received %q", item.Path, decision.Path))
	}
	switch decision.Status {
	case "write":
		if decision.Content == "" {
			violations = append(violations, "file worker write requires non-empty complete content")
		}
		if !strings.HasSuffix(decision.Content, "\n") {
			violations = append(violations, "file worker write requires complete content ending with a newline")
		}
		if strings.TrimSpace(decision.Error) != "" {
			violations = append(violations, "file worker write requires an empty error")
		}
		if item.Discipline != artifacts.ImplementationDisciplineDocumentation && implementationPlaceholderPattern.MatchString(decision.Content) {
			violations = append(violations, "file worker content contains a forbidden placeholder")
		}
		if implementationModulePlaceholderPattern.MatchString(decision.Content) {
			violations = append(violations, "file worker content contains a forbidden module-path placeholder")
		}
	case "satisfied":
		if decision.Content != "" || strings.TrimSpace(decision.Error) != "" {
			violations = append(violations, "file worker satisfied response requires empty content and error")
		}
	case "blocked":
		violations = append(violations, "file workers cannot declare blockers; the server validates dependencies before dispatch, so return a complete write or satisfied response")
	default:
		violations = append(violations, fmt.Sprintf("file worker returned invalid status %q", decision.Status))
	}
	if len(violations) > 0 {
		return decision, fmt.Errorf("invalid file worker decision: %s", strings.Join(violations, "; "))
	}
	return decision, nil
}

func parseImplementationReviewDecision(raw string, item artifacts.ImplementationWorkItem) (implementationReviewDecision, error) {
	var decision implementationReviewDecision
	if err := decodeStrictImplementationJSON(raw, &decision); err != nil {
		return decision, fmt.Errorf("decode file review decision: %w", err)
	}
	if decision.RoleID != "file_reviewer" {
		return decision, fmt.Errorf("file reviewer role drift: received %q", decision.RoleID)
	}
	if decision.WorkItemID != item.ID {
		return decision, fmt.Errorf("file reviewer work item drift: expected %q, received %q", item.ID, decision.WorkItemID)
	}
	decision.Findings = cleanOrderedStrings(decision.Findings)
	if len(decision.Findings) > 6 {
		return decision, fmt.Errorf("file reviewer exceeded the 6-finding limit")
	}
	switch decision.Verdict {
	case "pass":
		if len(decision.Findings) != 0 {
			return decision, fmt.Errorf("file reviewer pass verdict requires no findings")
		}
	case "revise":
		if len(decision.Findings) == 0 {
			return decision, fmt.Errorf("file reviewer revise verdict requires specific findings")
		}
	default:
		return decision, fmt.Errorf("file reviewer returned invalid verdict %q", decision.Verdict)
	}
	return decision, nil
}

func parseImplementationTriageDecision(raw string, ledger artifacts.ImplementationLedgerArtifact, verification artifacts.ImplementationWorkItem) (implementationTriageDecision, error) {
	var decision implementationTriageDecision
	if err := decodeStrictImplementationJSON(raw, &decision); err != nil {
		return decision, fmt.Errorf("decode failure triage decision: %w", err)
	}
	if decision.RoleID != "failure_triager" {
		return decision, fmt.Errorf("failure triager role drift: received %q", decision.RoleID)
	}
	if decision.VerificationItemID != verification.ID {
		return decision, fmt.Errorf("failure triager verification item drift: expected %q, received %q", verification.ID, decision.VerificationItemID)
	}
	byID := implementationItemIndexes(ledger)
	ownerIndex, exists := byID[decision.OwnerID]
	if !exists || ledger.Items[ownerIndex].Kind != artifacts.ImplementationWorkKindFile {
		return decision, fmt.Errorf("failure triager must select an existing file owner; received %q", decision.OwnerID)
	}
	closure := implementationDependencyClosure(verification, ledger, byID)
	if _, authorized := closure[decision.OwnerID]; !authorized {
		return decision, fmt.Errorf("failure triager selected owner %q outside the verification dependency graph", decision.OwnerID)
	}
	if strings.TrimSpace(decision.Feedback) == "" {
		return decision, fmt.Errorf("failure triager must provide direct corrective feedback")
	}
	return decision, nil
}

func decodeStrictImplementationJSON(raw string, output any) error {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}
