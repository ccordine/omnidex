package exactjson

import "testing"

type exactFixture struct {
	Name   string `json:"name"`
	Nested []struct {
		Digest string `json:"digest"`
	} `json:"nested"`
}

type opaqueFixture struct{}

func (*opaqueFixture) UnmarshalJSON([]byte) error { return nil }

func TestValidateObjectRejectsAmbiguousAuthorityRecursively(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"root duplicate":   `{"name":"a","name":"b","nested":[]}`,
		"nested duplicate": `{"name":"a","nested":[{"digest":"a","digest":"b"}]}`,
		"case alias":       `{"Name":"a","nested":[]}`,
		"nested alias":     `{"name":"a","nested":[{"Digest":"a"}]}`,
		"unknown":          `{"name":"a","nested":[],"other":true}`,
		"trailing":         `{"name":"a","nested":[]} {}`,
	} {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateObject([]byte(raw), exactFixture{}, "fixture"); err == nil {
				t.Fatal("ambiguous JSON authority was accepted")
			}
		})
	}
	if err := ValidateObject(
		[]byte(`{"name":"a","nested":[{"digest":"b"}]}`), exactFixture{}, "fixture",
	); err != nil {
		t.Fatalf("exact object: %v", err)
	}
}

func TestValidateCompatibleObjectAllowsUnrelatedMetadataButRejectsAliases(t *testing.T) {
	t.Parallel()
	if err := ValidateCompatibleObject(
		[]byte(`{"name":"a","nested":[{"digest":"b","provider_field":true}],"done":true}`),
		exactFixture{}, "fixture",
	); err != nil {
		t.Fatalf("compatible object: %v", err)
	}
	for _, raw := range []string{
		`{"Name":"a","nested":[]}`,
		`{"name":"a","nested":[{"Digest":"b"}]}`,
	} {
		if err := ValidateCompatibleObject([]byte(raw), exactFixture{}, "fixture"); err == nil {
			t.Fatal("compatible object accepted a case-folded typed alias")
		}
	}
}

func TestValidateObjectLeavesCustomNestedContractsOpaqueButStillRejectsDuplicates(t *testing.T) {
	t.Parallel()
	target := struct {
		Payload opaqueFixture `json:"payload"`
	}{}
	if err := ValidateObject(
		[]byte(`{"payload":{"registered":"value","nested":{"count":1}}}`), target, "fixture",
	); err != nil {
		t.Fatalf("opaque custom payload: %v", err)
	}
	if err := ValidateObject(
		[]byte(`{"payload":{"registered":"first","registered":"second"}}`), target, "fixture",
	); err == nil {
		t.Fatal("opaque custom payload bypassed recursive duplicate rejection")
	}
}
