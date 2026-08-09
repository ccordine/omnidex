package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const EvidencePackSchemaV2 = "omnidex.repository-evidence-pack.v2"

type Operation string

const (
	OperationSemanticExcerpts  Operation = "semantic_excerpts"
	OperationSymbolDeclaration Operation = "symbol_declaration"
	OperationDirectReferences  Operation = "direct_references"
)

func SupportedOperations() []Operation {
	return []Operation{
		OperationSemanticExcerpts,
		OperationSymbolDeclaration,
		OperationDirectReferences,
	}
}

func (operation Operation) Validate() error {
	switch operation {
	case OperationSemanticExcerpts, OperationSymbolDeclaration, OperationDirectReferences:
		return nil
	default:
		return fmt.Errorf("repository retrieval operation %q is unsupported", operation)
	}
}

type Limits struct {
	MaxSymbols   int `json:"max_symbols"`
	MaxEdges     int `json:"max_edges"`
	MaxSpanBytes int `json:"max_span_bytes"`
	MaxPackBytes int `json:"max_pack_bytes"`
}

type Request struct {
	ProjectID  int64
	AnalysisID string
	Operation  Operation
	Query      string
	Limits     Limits
}

type EvidenceSymbol struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Name         string  `json:"name"`
	Signature    string  `json:"signature"`
	SourceSHA256 string  `json:"source_sha256"`
	Source       string  `json:"source,omitempty"`
	MatchScore   float64 `json:"match_score,omitempty"`
}

type EvidenceRelation struct {
	ID         string  `json:"id"`
	FromID     string  `json:"from_id"`
	ToID       string  `json:"to_id"`
	Kind       string  `json:"kind"`
	Origin     string  `json:"origin"`
	Confidence float64 `json:"confidence"`
}

type SourceOmission struct {
	SymbolID string `json:"symbol_id"`
	Reason   string `json:"reason"`
}

type EvidencePack struct {
	Schema           string             `json:"schema"`
	ID               string             `json:"id"`
	SnapshotID       string             `json:"snapshot_id"`
	AnalysisID       string             `json:"analysis_id"`
	Operation        Operation          `json:"operation"`
	QueryBinding     string             `json:"query_binding"`
	SubjectSymbolID  string             `json:"subject_symbol_id,omitempty"`
	Symbols          []EvidenceSymbol   `json:"symbols"`
	Relations        []EvidenceRelation `json:"relations"`
	SourceOmissions  []SourceOmission   `json:"source_omissions"`
	OmittedSymbolIDs []string           `json:"omitted_symbol_ids"`
	OmittedEdges     int                `json:"omitted_edges"`
	MaxBytes         int                `json:"max_bytes"`
}

func FinalizeEvidencePack(pack *EvidencePack) error {
	if pack == nil {
		return fmt.Errorf("repository evidence pack is required")
	}
	id, err := evidencePackID(*pack)
	if err != nil {
		return err
	}
	pack.ID = id
	return pack.Validate()
}

func (limits Limits) validate() error {
	if limits.MaxSymbols < 1 || limits.MaxSymbols > 20 {
		return fmt.Errorf("repository evidence max symbols must be between 1 and 20")
	}
	if limits.MaxEdges < 1 || limits.MaxEdges > 100 {
		return fmt.Errorf("repository evidence max edges must be between 1 and 100")
	}
	if limits.MaxSpanBytes < 256 || limits.MaxSpanBytes > 64*1024 {
		return fmt.Errorf("repository evidence max span bytes must be between 256 and 65536")
	}
	if limits.MaxPackBytes < 1024 || limits.MaxPackBytes > 64*1024 {
		return fmt.Errorf("repository evidence max pack bytes must be between 1024 and 65536")
	}
	return nil
}

func (pack EvidencePack) Validate() error {
	if pack.Schema != EvidencePackSchemaV2 || !validPackID(pack.ID) {
		return fmt.Errorf("repository evidence pack has invalid identity")
	}
	if err := pack.Operation.Validate(); err != nil {
		return err
	}
	if !validQueryBinding(pack.QueryBinding) {
		return fmt.Errorf("repository evidence pack requires one opaque query binding")
	}
	if strings.TrimSpace(pack.SnapshotID) == "" || strings.TrimSpace(pack.AnalysisID) == "" || len(pack.Symbols) == 0 {
		return fmt.Errorf("repository evidence pack requires snapshot, analysis, and symbol evidence")
	}
	if pack.Symbols == nil || pack.Relations == nil || pack.SourceOmissions == nil || pack.OmittedSymbolIDs == nil {
		return fmt.Errorf("repository evidence pack requires canonical non-nil collections")
	}
	if err := pack.validateOperationProjection(); err != nil {
		return err
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	if len(raw) > pack.MaxBytes {
		return fmt.Errorf("repository evidence pack is %d bytes and exceeds %d", len(raw), pack.MaxBytes)
	}
	expected, err := evidencePackID(pack)
	if err != nil {
		return err
	}
	if pack.ID != expected {
		return fmt.Errorf("repository evidence pack ID does not match its exact projection")
	}
	return nil
}

// ValidateForRequest proves that an evidence projection was built for the
// exact typed retrieval request without carrying the plaintext query into the
// model-visible evidence pack.
func (pack EvidencePack) ValidateForRequest(operation Operation, query string) error {
	if err := pack.Validate(); err != nil {
		return err
	}
	binding, err := NewQueryBinding(operation, query)
	if err != nil {
		return err
	}
	if pack.Operation != operation || pack.QueryBinding != binding {
		return fmt.Errorf("repository evidence does not match its typed retrieval request")
	}
	return nil
}

func (pack EvidencePack) validateOperationProjection() error {
	symbols := make(map[string]struct{}, len(pack.Symbols))
	for _, symbol := range pack.Symbols {
		if strings.TrimSpace(symbol.ID) == "" {
			return fmt.Errorf("repository evidence pack contains an empty symbol ID")
		}
		if _, duplicate := symbols[symbol.ID]; duplicate {
			return fmt.Errorf("repository evidence pack contains duplicate symbol %q", symbol.ID)
		}
		symbols[symbol.ID] = struct{}{}
	}
	if pack.OmittedEdges < 0 {
		return fmt.Errorf("repository evidence pack omitted edge count cannot be negative")
	}
	omitted := make(map[string]struct{}, len(pack.OmittedSymbolIDs))
	for _, symbolID := range pack.OmittedSymbolIDs {
		if strings.TrimSpace(symbolID) == "" {
			return fmt.Errorf("repository evidence pack contains an empty omitted symbol ID")
		}
		if _, duplicate := omitted[symbolID]; duplicate {
			return fmt.Errorf("repository evidence pack contains duplicate omitted symbol %q", symbolID)
		}
		if _, included := symbols[symbolID]; included {
			return fmt.Errorf("repository evidence symbol %q cannot be included and omitted", symbolID)
		}
		omitted[symbolID] = struct{}{}
	}
	sourceOmissions := make(map[string]struct{}, len(pack.SourceOmissions))
	for _, omission := range pack.SourceOmissions {
		if _, exists := symbols[omission.SymbolID]; !exists {
			return fmt.Errorf("repository source omission names unavailable symbol %q", omission.SymbolID)
		}
		if omission.Reason != "source_span_exceeds_limit" {
			return fmt.Errorf("repository source omission for %q has unsupported reason %q", omission.SymbolID, omission.Reason)
		}
		if _, duplicate := sourceOmissions[omission.SymbolID]; duplicate {
			return fmt.Errorf("repository source omission for %q is duplicated", omission.SymbolID)
		}
		sourceOmissions[omission.SymbolID] = struct{}{}
	}
	relations := make(map[string]struct{}, len(pack.Relations))
	for _, relation := range pack.Relations {
		if _, duplicate := relations[relation.ID]; duplicate || strings.TrimSpace(relation.ID) == "" {
			return fmt.Errorf("repository evidence relation %q is empty or duplicated", relation.ID)
		}
		relations[relation.ID] = struct{}{}
		if _, exists := symbols[relation.FromID]; !exists {
			return fmt.Errorf("repository evidence relation %q has no source symbol", relation.ID)
		}
		if _, exists := symbols[relation.ToID]; !exists {
			return fmt.Errorf("repository evidence relation %q has no target symbol", relation.ID)
		}
	}
	switch pack.Operation {
	case OperationSemanticExcerpts:
		if pack.SubjectSymbolID != "" {
			return fmt.Errorf("semantic repository evidence cannot declare an exact subject symbol")
		}
	case OperationSymbolDeclaration:
		if len(pack.Symbols) != 1 || pack.SubjectSymbolID != pack.Symbols[0].ID || len(pack.Relations) != 0 || len(pack.OmittedSymbolIDs) != 0 || pack.OmittedEdges != 0 {
			return fmt.Errorf("symbol declaration evidence requires exactly one subject declaration and no relations")
		}
	case OperationDirectReferences:
		if _, exists := symbols[pack.SubjectSymbolID]; !exists {
			return fmt.Errorf("direct reference evidence requires its exact subject symbol")
		}
		for _, relation := range pack.Relations {
			if relation.ToID != pack.SubjectSymbolID {
				return fmt.Errorf("direct reference relation %q is not incoming to its subject", relation.ID)
			}
			if _, exists := symbols[relation.FromID]; !exists {
				return fmt.Errorf("direct reference relation %q has no caller evidence", relation.ID)
			}
		}
	}
	return nil
}

func evidencePackID(pack EvidencePack) (string, error) {
	pack.ID = ""
	raw, err := json.Marshal(pack)
	if err != nil {
		return "", fmt.Errorf("encode repository evidence pack identity: %w", err)
	}
	hash := sha256.Sum256(raw)
	return "evidence_pack_" + hex.EncodeToString(hash[:]), nil
}

func validPackID(value string) bool {
	if !strings.HasPrefix(value, "evidence_pack_") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "evidence_pack_"))
	return err == nil && len(decoded) == sha256.Size
}
