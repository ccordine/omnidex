package datasource

import (
	"bytes"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"
)

const (
	postgresOIDBool        = 16
	postgresOIDBytea       = 17
	postgresOIDInt8        = 20
	postgresOIDInt2        = 21
	postgresOIDInt4        = 23
	postgresOIDText        = 25
	postgresOIDJSON        = 114
	postgresOIDFloat4      = 700
	postgresOIDFloat8      = 701
	postgresOIDDate        = 1082
	postgresOIDTimestamp   = 1114
	postgresOIDTimestamptz = 1184
	postgresOIDNumeric     = 1700
	postgresOIDUUID        = 2950
	postgresOIDJSONB       = 3802
)

func normalizeEvidenceValue(oid uint32, value any) (EvidenceValue, error) {
	if value == nil {
		return EvidenceValue{Kind: EvidenceNull}, nil
	}
	switch oid {
	case postgresOIDBool:
		boolean, ok := value.(bool)
		if !ok {
			return EvidenceValue{}, fmt.Errorf("PostgreSQL boolean decoded as %T", value)
		}
		return EvidenceValue{Kind: EvidenceBoolean, Value: strconv.FormatBool(boolean)}, nil
	case postgresOIDInt2, postgresOIDInt4, postgresOIDInt8:
		return EvidenceValue{Kind: EvidenceInteger, Value: fmt.Sprint(value)}, nil
	case postgresOIDFloat4, postgresOIDFloat8, postgresOIDNumeric:
		text, err := canonicalDriverValue(value)
		if err != nil {
			return EvidenceValue{}, err
		}
		return EvidenceValue{Kind: EvidenceDecimal, Value: text}, nil
	case postgresOIDDate:
		if instant, ok := value.(time.Time); ok {
			return EvidenceValue{Kind: EvidenceDate, Value: instant.Format("2006-01-02")}, nil
		}
		return EvidenceValue{Kind: EvidenceDate, Value: fmt.Sprint(value)}, nil
	case postgresOIDTimestamp, postgresOIDTimestamptz:
		instant, ok := value.(time.Time)
		if !ok {
			return EvidenceValue{}, fmt.Errorf("PostgreSQL timestamp decoded as %T", value)
		}
		return EvidenceValue{Kind: EvidenceTimestamp, Value: instant.UTC().Format(time.RFC3339Nano)}, nil
	case postgresOIDUUID:
		return EvidenceValue{Kind: EvidenceUUID, Value: formatUUIDValue(value)}, nil
	case postgresOIDJSON, postgresOIDJSONB:
		canonical, err := canonicalJSONValue(value)
		if err != nil {
			return EvidenceValue{}, err
		}
		return EvidenceValue{Kind: EvidenceJSON, Value: canonical}, nil
	case postgresOIDBytea:
		bytes, ok := value.([]byte)
		if !ok {
			return EvidenceValue{}, fmt.Errorf("PostgreSQL bytea decoded as %T", value)
		}
		return EvidenceValue{Kind: EvidenceBinary, Value: base64.StdEncoding.EncodeToString(bytes)}, nil
	default:
		switch typed := value.(type) {
		case string:
			return EvidenceValue{Kind: EvidenceText, Value: typed}, nil
		case []byte:
			if !utf8.Valid(typed) {
				return EvidenceValue{Kind: EvidenceBinary, Value: base64.StdEncoding.EncodeToString(typed)}, nil
			}
			return EvidenceValue{Kind: EvidenceText, Value: string(typed)}, nil
		default:
			text, err := canonicalDriverValue(value)
			if err != nil {
				return EvidenceValue{}, err
			}
			return EvidenceValue{Kind: EvidenceText, Value: text}, nil
		}
	}
}

func canonicalDriverValue(value any) (string, error) {
	if valuer, ok := value.(driver.Valuer); ok {
		resolved, err := valuer.Value()
		if err != nil {
			return "", fmt.Errorf("encode PostgreSQL value: %w", err)
		}
		return fmt.Sprint(resolved), nil
	}
	return fmt.Sprint(value), nil
}

func canonicalJSONValue(value any) (string, error) {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		raw = encoded
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode PostgreSQL JSON: %w", err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("encode canonical PostgreSQL JSON: %w", err)
	}
	return string(encoded), nil
}

func formatUUIDValue(value any) string {
	switch typed := value.(type) {
	case [16]byte:
		hexValue := hex.EncodeToString(typed[:])
		return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32]
	case []byte:
		if len(typed) == 16 {
			var array [16]byte
			copy(array[:], typed)
			return formatUUIDValue(array)
		}
	}
	return fmt.Sprint(value)
}

func finalizeTypedResult(columns []EvidenceColumn, rows [][]EvidenceValue, byteCount int) (TypedEvidenceResult, error) {
	canonical := struct {
		Columns []EvidenceColumn  `json:"columns"`
		Rows    [][]EvidenceValue `json:"rows"`
	}{Columns: columns, Rows: rows}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return TypedEvidenceResult{}, fmt.Errorf("encode typed evidence result: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return TypedEvidenceResult{Columns: columns, Rows: rows, RowCount: len(rows), ByteCount: byteCount, Hash: hex.EncodeToString(digest[:])}, nil
}
