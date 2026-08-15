package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const AcceptanceObservationInventorySchemaV1 = "omnidex.acceptance-observation-inventory.v1"

type AcceptanceObservationLiteral struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type AcceptanceObservationStatement struct {
	ID            string   `json:"statement_id"`
	StatementKind string   `json:"statement_kind"`
	Structure     []string `json:"structure"`
	Operators     []string `json:"operators"`
}

type AcceptanceObservationSite struct {
	ID            string                         `json:"site_id"`
	AssertionID   string                         `json:"assertion_id"`
	StatementID   string                         `json:"statement_id"`
	StatementKind string                         `json:"statement_kind"`
	Structure     []string                       `json:"structure"`
	Operations    []string                       `json:"operations"`
	Operators     []string                       `json:"operators"`
	Literals      []AcceptanceObservationLiteral `json:"literals"`
}

type AcceptanceObservationLocator struct {
	SiteID      string `json:"site_id"`
	StartByte   int    `json:"start_byte"`
	EndByte     int    `json:"end_byte"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	EndLine     int    `json:"end_line"`
	EndColumn   int    `json:"end_column"`
}

type AcceptanceObservationInventory struct {
	Schema          string                           `json:"schema"`
	InventorySHA256 string                           `json:"inventory_sha256"`
	Statements      []AcceptanceObservationStatement `json:"statements"`
	Sites           []AcceptanceObservationSite      `json:"sites"`
	Locators        []AcceptanceObservationLocator   `json:"locators"`
}

type acceptanceObservationInventoryDigest struct {
	Schema     string                           `json:"schema"`
	Statements []AcceptanceObservationStatement `json:"statements"`
	Sites      []AcceptanceObservationSite      `json:"sites"`
	Locators   []AcceptanceObservationLocator   `json:"locators"`
}

func InventoryTypeScriptAcceptanceObservations(
	source string,
	tsx bool,
) (AcceptanceObservationInventory, error) {
	var zero AcceptanceObservationInventory
	if strings.TrimSpace(source) == "" {
		return zero, fmt.Errorf("acceptance observation inventory requires source")
	}
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return zero, err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return zero, fmt.Errorf("acceptance observation source contains invalid TypeScript syntax")
	}
	declaration, err := soleAcceptanceFunction(root)
	if err != nil {
		return zero, err
	}
	body := declaration.ChildByFieldName("body")
	if body == nil || body.Kind() != "statement_block" {
		return zero, fmt.Errorf("acceptance observation source function has no statement body")
	}
	inventory := AcceptanceObservationInventory{
		Schema:     AcceptanceObservationInventorySchemaV1,
		Statements: []AcceptanceObservationStatement{}, Sites: []AcceptanceObservationSite{},
		Locators: []AcceptanceObservationLocator{},
	}
	bytes := []byte(source)
	grammar := analyzeAcceptanceObserverGrammar(declaration, bytes)
	for index := uint(0); index < body.NamedChildCount(); index++ {
		statement := body.NamedChild(index)
		if statement == nil || statement.Kind() == "comment" {
			continue
		}
		statementID := fmt.Sprintf("statement_%03d", len(inventory.Statements)+1)
		inventory.Statements = append(inventory.Statements, newAcceptanceObservationStatement(
			statementID, statement, bytes,
		))
		candidates := collectAcceptanceCallCandidates(statement, bytes)
		for candidateIndex := range candidates {
			if _, trusted := grammar.trustedCalls[candidates[candidateIndex].node.Id()]; !trusted {
				candidates[candidateIndex].untrusted = true
			}
		}
		if _, allowed := grammar.allowedStatements[statement.Id()]; !allowed {
			candidates = append([]acceptanceObservationCandidate{{
				node: statement, operation: "untrusted_call", residual: true,
			}}, candidates...)
		}
		for _, candidate := range candidates {
			sequence := len(inventory.Sites) + 1
			site := newAcceptanceObservationSite(
				fmt.Sprintf("site_%03d", sequence), fmt.Sprintf("assertion_%03d", sequence),
				statementID, statement.Kind(), candidate, bytes,
			)
			inventory.Sites = append(inventory.Sites, site)
			inventory.Locators = append(inventory.Locators, acceptanceObservationNodeLocator(site.ID, candidate.node))
		}
	}
	if len(inventory.Statements) == 0 || len(inventory.Sites) == 0 {
		return zero, fmt.Errorf("acceptance observation source has no executable observation sites")
	}
	digest, err := acceptanceObservationInventorySHA(inventory)
	if err != nil {
		return zero, err
	}
	inventory.InventorySHA256 = digest
	return inventory, nil
}

func acceptanceObservationInventorySHA(inventory AcceptanceObservationInventory) (string, error) {
	raw, err := exactjson.Canonical(acceptanceObservationInventoryDigest{
		Schema: inventory.Schema, Statements: inventory.Statements,
		Sites: inventory.Sites, Locators: inventory.Locators,
	})
	if err != nil {
		return "", fmt.Errorf("canonicalize acceptance observation inventory: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (inventory AcceptanceObservationInventory) canonicalModelProjection() string {
	raw, _ := json.Marshal(struct {
		Statements []AcceptanceObservationStatement `json:"statements"`
		Sites      []AcceptanceObservationSite      `json:"sites"`
	}{Statements: inventory.Statements, Sites: inventory.Sites})
	return string(raw)
}
