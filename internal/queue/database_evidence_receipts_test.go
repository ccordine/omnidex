package queue

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseEvidenceReceiptValidationMatchesHardExecutionBounds(t *testing.T) {
	t.Parallel()
	valid := DatabaseEvidenceReceipt{
		JobID: 1, DataSourceID: "source-1",
		SchemaFingerprint: strings.Repeat("a", 64),
		IntentHash:        strings.Repeat("b", 64),
		QueryHash:         strings.Repeat("c", 64),
		ResultHash:        strings.Repeat("d", 64),
		PlanTotalCost:     datasource.MaxEvidencePlanCost,
		PlanEstimatedRows: datasource.MaxEvidencePlanRows,
		ReturnedRows:      datasource.MaxIntentRows,
		ResultBytes:       datasource.MaxEvidenceResultBytes,
		AcquiredAt:        time.Unix(1_700_000_000, 0).UTC(),
	}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}

	invalid := map[string]func(*DatabaseEvidenceReceipt){
		"source identity": func(value *DatabaseEvidenceReceipt) { value.DataSourceID = "Invalid source" },
		"NaN cost":        func(value *DatabaseEvidenceReceipt) { value.PlanTotalCost = math.NaN() },
		"infinite cost":   func(value *DatabaseEvidenceReceipt) { value.PlanTotalCost = math.Inf(1) },
		"cost bound":      func(value *DatabaseEvidenceReceipt) { value.PlanTotalCost++ },
		"plan row bound":  func(value *DatabaseEvidenceReceipt) { value.PlanEstimatedRows++ },
		"result row bound": func(value *DatabaseEvidenceReceipt) {
			value.ReturnedRows++
		},
		"result byte bound": func(value *DatabaseEvidenceReceipt) {
			value.ResultBytes++
		},
		"uppercase hash": func(value *DatabaseEvidenceReceipt) {
			value.QueryHash = strings.Repeat("A", 64)
		},
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.validate(); err == nil {
				t.Fatalf("accepted invalid receipt: %+v", candidate)
			}
		})
	}
}

func TestDatabaseEvidenceResultValidationRejectsForgedHashAndByteCount(t *testing.T) {
	t.Parallel()
	evidence := databaseEvidenceResultFixture(t, "source-1", strings.Repeat("e", 64))
	if err := validateExactDatabaseEvidenceResult(evidence); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*datasource.EvidenceResult){
		"schema": func(value *datasource.EvidenceResult) { value.Schema = "invented" },
		"row count": func(value *datasource.EvidenceResult) {
			value.Result.RowCount++
		},
		"result hash": func(value *datasource.EvidenceResult) {
			value.Result.Rows[0][0].Value = "8"
		},
		"byte count": func(value *datasource.EvidenceResult) {
			value.Result.ByteCount++
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := evidence
			candidate.Result.Columns = append([]datasource.EvidenceColumn(nil), evidence.Result.Columns...)
			candidate.Result.Rows = make([][]datasource.EvidenceValue, len(evidence.Result.Rows))
			for index := range evidence.Result.Rows {
				candidate.Result.Rows[index] = append([]datasource.EvidenceValue(nil), evidence.Result.Rows[index]...)
			}
			mutate(&candidate)
			if err := validateExactDatabaseEvidenceResult(candidate); err == nil {
				t.Fatal("forged typed evidence was accepted")
			}
		})
	}
}
