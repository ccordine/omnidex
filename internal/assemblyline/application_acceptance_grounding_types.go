package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const AcceptanceGroundingReceiptSchemaV1 = "omnidex.application-acceptance-grounding-receipt.v1"

type AcceptanceGroundingAuthorityKind string

const (
	AcceptanceGroundingCriterion         AcceptanceGroundingAuthorityKind = "criterion"
	AcceptanceGroundingPlatformInvariant AcceptanceGroundingAuthorityKind = "platform_invariant"
)

type FrozenAcceptanceCriterion struct {
	ID        string `json:"criterion_id"`
	Statement string `json:"statement"`
}

type AcceptanceGroundingAuthority struct {
	ID         string                           `json:"authority_id"`
	Kind       AcceptanceGroundingAuthorityKind `json:"kind"`
	Statement  string                           `json:"statement"`
	Operations []string                         `json:"operations"`
}

type ApplicationAcceptanceGroundingReviewInput struct {
	WorkloadSHA256      string                         `json:"workload_sha256"`
	TaskID              string                         `json:"task_id"`
	SourceSHA256        string                         `json:"source_sha256"`
	TSX                 bool                           `json:"tsx"`
	Inventory           AcceptanceObservationInventory `json:"inventory"`
	Criteria            []FrozenAcceptanceCriterion    `json:"criteria"`
	PlatformAuthorities []AcceptanceGroundingAuthority `json:"platform_authorities"`
}

type AcceptanceGroundingDecision string

const (
	AcceptanceGroundingAccept AcceptanceGroundingDecision = "accept"
	AcceptanceGroundingRepair AcceptanceGroundingDecision = "repair"
)

type AcceptanceGroundingMapping struct {
	SiteID       string   `json:"site_id"`
	AuthorityIDs []string `json:"authority_ids"`
}

type ApplicationAcceptanceGroundingReview struct {
	Decision           AcceptanceGroundingDecision  `json:"decision"`
	Mappings           []AcceptanceGroundingMapping `json:"mappings,omitempty"`
	UnsupportedSiteID  string                       `json:"unsupported_site_id,omitempty"`
	MissingCriterionID string                       `json:"missing_criterion_id,omitempty"`

	binding string
}

type ApplicationAcceptanceGroundingReceipt struct {
	Schema          string                       `json:"schema"`
	WorkloadSHA256  string                       `json:"workload_sha256"`
	TaskID          string                       `json:"task_id"`
	SourceSHA256    string                       `json:"source_sha256"`
	InventorySHA256 string                       `json:"inventory_sha256"`
	Mappings        []AcceptanceGroundingMapping `json:"mappings"`
	BindingSHA256   string                       `json:"binding_sha256"`
}

var acceptanceAuthorityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

func NewApplicationAcceptanceGroundingReviewInput(
	context ApplicationTaskContext,
	source string,
	tsx bool,
	platformAuthorities []AcceptanceGroundingAuthority,
) (ApplicationAcceptanceGroundingReviewInput, error) {
	var zero ApplicationAcceptanceGroundingReviewInput
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, tsx)
	if err != nil {
		return zero, err
	}
	criteria := make([]FrozenAcceptanceCriterion, len(context.Task.AcceptanceCriteria))
	for index, criterion := range context.Task.AcceptanceCriteria {
		criteria[index] = FrozenAcceptanceCriterion{
			ID: fmt.Sprintf("criterion_%03d", index+1), Statement: criterion,
		}
	}
	input := ApplicationAcceptanceGroundingReviewInput{
		WorkloadSHA256: context.WorkloadSHA256, TaskID: context.Task.TaskID,
		SourceSHA256: acceptanceGroundingSourceSHA(source), TSX: tsx, Inventory: inventory,
		Criteria:            criteria,
		PlatformAuthorities: append([]AcceptanceGroundingAuthority(nil), platformAuthorities...),
	}
	if err := input.validate(); err != nil {
		return zero, err
	}
	return input, nil
}

func (input ApplicationAcceptanceGroundingReviewInput) validate() error {
	if err := validateAcceptanceSHA("workload", input.WorkloadSHA256); err != nil {
		return err
	}
	if input.TaskID == "" || input.TaskID != strings.TrimSpace(input.TaskID) ||
		!acceptanceAuthorityIDPattern.MatchString(input.TaskID) {
		return fmt.Errorf("acceptance grounding task identity is invalid")
	}
	if err := validateAcceptanceSHA("source", input.SourceSHA256); err != nil {
		return err
	}
	if err := validateAcceptanceObservationInventory(input.Inventory); err != nil {
		return err
	}
	if len(input.Criteria) == 0 || len(input.Criteria) > maxApplicationAcceptanceCriteria {
		return fmt.Errorf("acceptance grounding requires 1..%d frozen criteria", maxApplicationAcceptanceCriteria)
	}
	known := make(map[string]struct{}, len(input.Criteria)+len(input.PlatformAuthorities))
	for index, criterion := range input.Criteria {
		wantID := fmt.Sprintf("criterion_%03d", index+1)
		if criterion.ID != wantID {
			return fmt.Errorf("frozen acceptance criterion %d identity must be %q", index, wantID)
		}
		if err := validateAcceptanceStatement("criterion", criterion.Statement); err != nil {
			return err
		}
		known[criterion.ID] = struct{}{}
	}
	for index, authority := range input.PlatformAuthorities {
		if authority.Kind != AcceptanceGroundingPlatformInvariant {
			return fmt.Errorf("platform authority %d must have platform_invariant kind", index)
		}
		if !strings.HasPrefix(authority.ID, "platform_") || !acceptanceAuthorityIDPattern.MatchString(authority.ID) {
			return fmt.Errorf("platform authority %d identity is invalid", index)
		}
		if _, duplicate := known[authority.ID]; duplicate {
			return fmt.Errorf("acceptance grounding authority %q is duplicated", authority.ID)
		}
		if err := validateAcceptanceStatement("platform authority", authority.Statement); err != nil {
			return err
		}
		if len(authority.Operations) == 0 {
			return fmt.Errorf("platform authority %s requires registered operations", authority.ID)
		}
		lastOperation := ""
		for _, operation := range authority.Operations {
			if !acceptancePlatformOperation(operation) {
				return fmt.Errorf("platform authority %s contains unsupported operation %q", authority.ID, operation)
			}
			if lastOperation != "" && operation <= lastOperation {
				return fmt.Errorf("platform authority %s operations must be unique and ordered", authority.ID)
			}
			for _, existing := range input.PlatformAuthorities[:index] {
				if stringInSet(operation, existing.Operations) {
					return fmt.Errorf("platform operation %s has multiple authorities", operation)
				}
			}
			lastOperation = operation
		}
		known[authority.ID] = struct{}{}
	}
	for _, site := range input.Inventory.Sites {
		operation, pure := acceptancePurePlatformSite(site)
		if pure && input.platformAuthorityFor(operation) == "" {
			return fmt.Errorf("acceptance harness operation %s has no registered platform authority", operation)
		}
	}
	if _, err := input.deterministicHarnessMappings(); err != nil {
		return err
	}
	return nil
}

func validateAcceptanceObservationInventory(inventory AcceptanceObservationInventory) error {
	if inventory.Schema != AcceptanceObservationInventorySchemaV1 ||
		len(inventory.Statements) == 0 || len(inventory.Sites) == 0 ||
		len(inventory.Locators) != len(inventory.Sites) {
		return fmt.Errorf("acceptance grounding requires a non-empty canonical observation inventory")
	}
	statements := make(map[string]struct{}, len(inventory.Statements))
	for index, statement := range inventory.Statements {
		wantID := fmt.Sprintf("statement_%03d", index+1)
		if statement.ID != wantID || statement.StatementKind == "comment" ||
			!acceptanceSyntaxKindPattern.MatchString(statement.StatementKind) || len(statement.Structure) == 0 {
			return fmt.Errorf("acceptance observation statement %d is invalid", index)
		}
		statements[statement.ID] = struct{}{}
	}
	for index, site := range inventory.Sites {
		if site.ID != fmt.Sprintf("site_%03d", index+1) ||
			site.AssertionID != fmt.Sprintf("assertion_%03d", index+1) {
			return fmt.Errorf("acceptance observation site %d has non-canonical identity", index)
		}
		if err := validateAcceptanceObservationSite(site); err != nil {
			return err
		}
		if _, exists := statements[site.StatementID]; !exists {
			return fmt.Errorf("acceptance observation site %s has unknown statement identity", site.ID)
		}
		locator := inventory.Locators[index]
		if locator.SiteID != site.ID || locator.StartByte < 0 || locator.EndByte <= locator.StartByte ||
			locator.StartLine < 1 || locator.StartColumn < 1 || locator.EndLine < locator.StartLine ||
			locator.EndColumn < 1 {
			return fmt.Errorf("acceptance observation site %s has invalid locator", site.ID)
		}
	}
	digest, err := acceptanceObservationInventorySHA(inventory)
	if err != nil {
		return err
	}
	if inventory.InventorySHA256 != digest {
		return fmt.Errorf("acceptance observation inventory hash does not match its canonical sites")
	}
	return nil
}

func validateAcceptanceStatement(label, statement string) error {
	if statement == "" || statement != strings.TrimSpace(statement) {
		return fmt.Errorf("acceptance grounding %s statement is empty or untrimmed", label)
	}
	if utf8.RuneCountInString(statement) > maxApplicationCriterionRunes {
		return fmt.Errorf("acceptance grounding %s statement exceeds %d runes", label, maxApplicationCriterionRunes)
	}
	return nil
}

func validateAcceptanceSHA(label, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return fmt.Errorf("acceptance grounding %s hash must be 64 lowercase hexadecimal characters", label)
	}
	return nil
}

func acceptanceGroundingSourceSHA(source string) string {
	digest := sha256.Sum256([]byte(source))
	return hex.EncodeToString(digest[:])
}

func (input ApplicationAcceptanceGroundingReviewInput) validateSource(source string) error {
	if acceptanceGroundingSourceSHA(source) != input.SourceSHA256 {
		return fmt.Errorf("acceptance grounding source differs from bound source")
	}
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, input.TSX)
	if err != nil {
		return err
	}
	if inventory.InventorySHA256 != input.Inventory.InventorySHA256 {
		return fmt.Errorf("acceptance grounding inventory differs from current source")
	}
	return nil
}

func acceptanceGroundingInputBinding(input ApplicationAcceptanceGroundingReviewInput) (string, error) {
	raw, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("canonicalize acceptance grounding authority: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
