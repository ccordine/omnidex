package datasource

import "testing"

func TestWantsTextAnalysis(t *testing.T) {
	if !WantsTextAnalysis("What is the sentiment in patient feedback comments this month?") {
		t.Fatal("expected sentiment question to match")
	}
	if WantsTextAnalysis("How many appointments tomorrow?") {
		t.Fatal("expected non-text question to not match")
	}
}

func TestIsTextContentColumn(t *testing.T) {
	if !IsTextContentColumn("patient_feedback", "text") {
		t.Fatal("patient_feedback should be text content")
	}
	if IsTextContentColumn("patient_name", "text") {
		t.Fatal("patient_name should not be text content")
	}
	if IsTextContentColumn("clinical_progress_note", "text") {
		t.Fatal("clinical note should not be patient submission field")
	}
}

func TestValidatePrivacyAllowsBoundedTextSample(t *testing.T) {
	sql := "SELECT feedback_comment FROM patient_surveys WHERE created_at >= NOW() - INTERVAL '30 days' LIMIT 20"
	if err := ValidatePrivacySafeSelect(sql, true); err != nil {
		t.Fatalf("bounded text sample should be allowed: %v", err)
	}
}

func TestValidatePrivacyBlocksUnboundedTextSelect(t *testing.T) {
	sql := "SELECT feedback_comment FROM patient_surveys"
	if err := ValidatePrivacySafeSelect(sql, true); err == nil {
		t.Fatal("unbounded text select should be blocked")
	}
}

func TestValidatePrivacyBlocksTextWithIdentifiers(t *testing.T) {
	sql := "SELECT patient_name, feedback_comment FROM patient_surveys LIMIT 10"
	if err := ValidatePrivacySafeSelect(sql, true); err == nil {
		t.Fatal("text sample with identifiers should be blocked")
	}
}

func TestRedactTextColumns(t *testing.T) {
	rows := []map[string]any{{"feedback_comment": "long wait time", "count": 1}}
	got := redactTextColumns(rows, []string{"feedback_comment"})
	if got[0]["feedback_comment"] != "[text analyzed — not shown]" {
		t.Fatalf("unexpected redaction: %#v", got[0]["feedback_comment"])
	}
	if got[0]["count"] != 1 {
		t.Fatalf("non-text column should remain: %#v", got[0]["count"])
	}
}
