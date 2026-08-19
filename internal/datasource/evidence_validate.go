package datasource

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

func (evidence EvidenceResult) ValidateForPlan(
	snapshot SchemaSnapshot,
	plan RelationalQueryPlan,
	limits ExecutionLimits,
) error {
	if err := plan.Validate(snapshot); err != nil {
		return err
	}
	if err := validateExecutionBounds(plan.Intent.Limit, limits); err != nil {
		return err
	}
	provenance := evidence.Provenance
	if evidence.Schema != EvidenceResultV1 || provenance.SourceID != plan.SourceID ||
		provenance.SchemaFingerprint != plan.SchemaFingerprint || provenance.IntentHash != plan.IntentHash {
		return fmt.Errorf("database evidence authority does not match relational plan")
	}
	for label, value := range map[string]string{
		"intent hash": provenance.IntentHash, "query hash": provenance.QueryHash,
		"result hash": provenance.ResultHash,
	} {
		if !exactSHA256(value) {
			return fmt.Errorf("database evidence %s is not an exact SHA-256", label)
		}
	}
	if math.IsNaN(provenance.Plan.TotalCost) || math.IsInf(provenance.Plan.TotalCost, 0) ||
		provenance.Plan.TotalCost < 0 || provenance.Plan.TotalCost > limits.MaxTotalCost ||
		provenance.Plan.EstimatedRows < 0 || provenance.Plan.EstimatedRows > limits.MaxPlanRows {
		return fmt.Errorf("database evidence execution plan exceeds its authorized bounds")
	}
	if provenance.AcquiredAt.IsZero() || provenance.AcquiredAt.Location() != time.UTC {
		return fmt.Errorf("database evidence requires one UTC acquisition time")
	}
	if err := validateTypedEvidenceResult(evidence.Result, plan.Outputs, limits); err != nil {
		return err
	}
	if provenance.ResultHash != evidence.Result.Hash {
		return fmt.Errorf("database evidence provenance does not bind its typed result")
	}
	return nil
}

func validateTypedEvidenceResult(
	result TypedEvidenceResult,
	outputs []CompiledOutput,
	limits ExecutionLimits,
) error {
	if len(result.Columns) == 0 || len(result.Columns) != len(outputs) ||
		result.RowCount != len(result.Rows) || result.RowCount > limits.MaxRows {
		return fmt.Errorf("database evidence result shape is outside its authorized bounds")
	}
	for index, column := range result.Columns {
		output := outputs[index]
		if column.Name != output.Name || column.FieldID != output.FieldID ||
			column.Aggregate != output.Aggregate || column.TypeCategory != output.TypeCategory {
			return fmt.Errorf("database evidence column %d does not match the relational plan", index+1)
		}
	}
	columns, err := json.Marshal(result.Columns)
	if err != nil {
		return fmt.Errorf("encode database evidence columns: %w", err)
	}
	byteCount := len(columns)
	for rowIndex, row := range result.Rows {
		if len(row) != len(result.Columns) {
			return fmt.Errorf("database evidence row %d has the wrong column count", rowIndex+1)
		}
		for columnIndex, value := range row {
			if err := validateEvidenceValue(value, result.Columns[columnIndex].TypeCategory); err != nil {
				return fmt.Errorf("database evidence row %d column %d: %w", rowIndex+1, columnIndex+1, err)
			}
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("encode database evidence row: %w", err)
		}
		byteCount += len(encoded)
	}
	if byteCount != result.ByteCount || byteCount > limits.MaxBytes {
		return fmt.Errorf("database evidence byte count is invalid or exceeds its authorized bound")
	}
	canonical, err := json.Marshal(struct {
		Columns []EvidenceColumn  `json:"columns"`
		Rows    [][]EvidenceValue `json:"rows"`
	}{Columns: result.Columns, Rows: result.Rows})
	if err != nil {
		return fmt.Errorf("encode database evidence result: %w", err)
	}
	digest := sha256.Sum256(canonical)
	if result.Hash != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("database evidence result hash does not match its typed rows")
	}
	return nil
}

func validateEvidenceValue(value EvidenceValue, category ColumnTypeCategory) error {
	if value.Kind == EvidenceNull {
		if value.Value != "" {
			return fmt.Errorf("null value must not carry content")
		}
		return nil
	}
	expected := map[ColumnTypeCategory]EvidenceValueKind{
		TypeInteger: EvidenceInteger, TypeDecimal: EvidenceDecimal, TypeText: EvidenceText,
		TypeBoolean: EvidenceBoolean, TypeTemporal: EvidenceTimestamp, TypeDate: EvidenceDate,
		TypeUUID: EvidenceUUID, TypeJSON: EvidenceJSON, TypeBinary: EvidenceBinary,
		TypeOther: EvidenceText,
	}[category]
	if expected == "" || value.Kind != expected {
		return fmt.Errorf("value kind %q does not match column category %q", value.Kind, category)
	}
	switch value.Kind {
	case EvidenceText:
		if !utf8.ValidString(value.Value) || strings.ContainsRune(value.Value, '\x00') {
			return fmt.Errorf("text value is not valid NUL-free UTF-8")
		}
	case EvidenceInteger:
		return validateLiteral(IntentLiteral{Type: LiteralInteger, Value: value.Value})
	case EvidenceDecimal:
		return validateLiteral(IntentLiteral{Type: LiteralDecimal, Value: value.Value})
	case EvidenceBoolean:
		return validateLiteral(IntentLiteral{Type: LiteralBoolean, Value: value.Value})
	case EvidenceTimestamp:
		return validateLiteral(IntentLiteral{Type: LiteralTimestamp, Value: value.Value})
	case EvidenceDate:
		return validateLiteral(IntentLiteral{Type: LiteralDate, Value: value.Value})
	case EvidenceUUID:
		return validateLiteral(IntentLiteral{Type: LiteralUUID, Value: value.Value})
	case EvidenceJSON:
		canonical, err := canonicalJSONValue(value.Value)
		if err != nil || canonical != value.Value {
			return fmt.Errorf("JSON value is not canonical")
		}
	case EvidenceBinary:
		decoded, err := base64.StdEncoding.DecodeString(value.Value)
		if err != nil || base64.StdEncoding.EncodeToString(decoded) != value.Value {
			return fmt.Errorf("binary value is not canonical base64")
		}
	}
	return nil
}

func exactSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
