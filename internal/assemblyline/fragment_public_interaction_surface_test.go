package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFragmentPublicInteractionSurfaceRendersUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		surface FragmentPublicInteractionSurface
		want    string
	}{
		{
			name: "inventory",
			surface: FragmentPublicInteractionSurface{
				Schema: FragmentPublicInteractionSurfaceSchemaV1,
				Controls: []FragmentPublicInteractionControl{
					{Role: FragmentPublicRoleTextbox, RoleOrdinal: 1, RoleCount: 2, AccessibleName: "Item name", PlaceholderHint: "Search stock", ValueKind: FragmentPublicValueText},
					{Role: FragmentPublicRoleSpinbutton, RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Quantity", ValueKind: FragmentPublicValueNumber},
					{Role: FragmentPublicRoleTextbox, RoleOrdinal: 2, RoleCount: 2, PlaceholderHint: "Storage bin", ValueKind: FragmentPublicValueText},
					{Role: FragmentPublicRoleButton, RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Save item", ValueKind: FragmentPublicValueAction},
				},
				Outputs: []FragmentPublicOutput{
					{AccessibleName: "Current quantity"},
				},
			},
			want: "PUBLIC_INTERACTION_SURFACE_V1\n" +
				"CONTROL 1 role=textbox role_ordinal=1 role_count=2 accessible_name=\"Item name\" placeholder_hint=\"Search stock\" value_kind=text\n" +
				"CONTROL 2 role=spinbutton role_ordinal=1 role_count=1 accessible_name=\"Quantity\" placeholder_hint=NONE value_kind=number\n" +
				"CONTROL 3 role=textbox role_ordinal=2 role_count=2 accessible_name=NONE placeholder_hint=\"Storage bin\" value_kind=text\n" +
				"CONTROL 4 role=button role_ordinal=1 role_count=1 accessible_name=\"Save item\" placeholder_hint=NONE value_kind=action\n" +
				"OUTPUT 1 role=status accessible_name=\"Current quantity\"\n" +
				"END_PUBLIC_INTERACTION_SURFACE",
		},
		{
			name: "travel",
			surface: FragmentPublicInteractionSurface{
				Schema: FragmentPublicInteractionSurfaceSchemaV1,
				Controls: []FragmentPublicInteractionControl{
					{Role: FragmentPublicRoleCombobox, RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Destination", ValueKind: FragmentPublicValueSelection},
					{Role: FragmentPublicRoleCheckbox, RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Direct trips only", ValueKind: FragmentPublicValueBoolean},
					{Role: FragmentPublicRoleButton, RoleOrdinal: 1, RoleCount: 2, AccessibleName: "Find trips", ValueKind: FragmentPublicValueAction},
					{Role: FragmentPublicRoleButton, RoleOrdinal: 2, RoleCount: 2, AccessibleName: "Clear search", ValueKind: FragmentPublicValueAction},
				},
				Outputs: []FragmentPublicOutput{
					{AccessibleName: "Journey duration"},
				},
			},
			want: "PUBLIC_INTERACTION_SURFACE_V1\n" +
				"CONTROL 1 role=combobox role_ordinal=1 role_count=1 accessible_name=\"Destination\" placeholder_hint=NONE value_kind=selection\n" +
				"CONTROL 2 role=checkbox role_ordinal=1 role_count=1 accessible_name=\"Direct trips only\" placeholder_hint=NONE value_kind=boolean\n" +
				"CONTROL 3 role=button role_ordinal=1 role_count=2 accessible_name=\"Find trips\" placeholder_hint=NONE value_kind=action\n" +
				"CONTROL 4 role=button role_ordinal=2 role_count=2 accessible_name=\"Clear search\" placeholder_hint=NONE value_kind=action\n" +
				"OUTPUT 1 role=status accessible_name=\"Journey duration\"\n" +
				"END_PUBLIC_INTERACTION_SURFACE",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := test.surface.Render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != test.want {
				t.Fatalf("receipt mismatch\ngot:\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

func TestFragmentPublicInteractionSurfaceValidatesRoleValueKinds(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		role FragmentPublicInteractionRole
		kind FragmentPublicInteractionValueKind
	}{
		{FragmentPublicRoleButton, FragmentPublicValueAction},
		{FragmentPublicRoleCheckbox, FragmentPublicValueBoolean},
		{FragmentPublicRoleCombobox, FragmentPublicValueSelection},
		{FragmentPublicRoleListbox, FragmentPublicValueSelection},
		{FragmentPublicRoleRadio, FragmentPublicValueSelection},
		{FragmentPublicRoleSearchbox, FragmentPublicValueText},
		{FragmentPublicRoleSlider, FragmentPublicValueNumber},
		{FragmentPublicRoleSpinbutton, FragmentPublicValueNumber},
		{FragmentPublicRoleTextbox, FragmentPublicValueText},
	}
	for _, pair := range pairs {
		surface := FragmentPublicInteractionSurface{
			Schema: FragmentPublicInteractionSurfaceSchemaV1,
			Controls: []FragmentPublicInteractionControl{{
				Role: pair.role, RoleOrdinal: 1, RoleCount: 1, ValueKind: pair.kind,
			}},
		}
		if err := surface.Validate(); err != nil {
			t.Fatalf("valid pair %s/%s: %v", pair.role, pair.kind, err)
		}
	}
}

func TestFragmentPublicInteractionSurfaceRejectsInvalidState(t *testing.T) {
	t.Parallel()
	base := FragmentPublicInteractionSurface{
		Schema: FragmentPublicInteractionSurfaceSchemaV1,
		Controls: []FragmentPublicInteractionControl{
			{Role: FragmentPublicRoleTextbox, RoleOrdinal: 1, RoleCount: 2, AccessibleName: "Stock query", ValueKind: FragmentPublicValueText},
			{Role: FragmentPublicRoleTextbox, RoleOrdinal: 2, RoleCount: 2, PlaceholderHint: "Second query", ValueKind: FragmentPublicValueText},
		},
		Outputs: []FragmentPublicOutput{
			{AccessibleName: "Stock total"},
			{AccessibleName: "Lot total"},
		},
	}
	tests := []struct {
		name   string
		mutate func(*FragmentPublicInteractionSurface)
	}{
		{"schema", func(value *FragmentPublicInteractionSurface) {
			value.Schema = "omnidex.fragment-public-interaction-surface.v2"
		}},
		{"role", func(value *FragmentPublicInteractionSurface) { value.Controls[0].Role = "link" }},
		{"value kind", func(value *FragmentPublicInteractionSurface) { value.Controls[0].ValueKind = FragmentPublicValueNumber }},
		{"role ordinal", func(value *FragmentPublicInteractionSurface) { value.Controls[1].RoleOrdinal = 1 }},
		{"role count", func(value *FragmentPublicInteractionSurface) { value.Controls[0].RoleCount = 1 }},
		{"oversized literal", func(value *FragmentPublicInteractionSurface) {
			value.Controls[0].AccessibleName = strings.Repeat("x", MaxFragmentPublicInteractionLiteralBytes+1)
		}},
		{"invalid utf8", func(value *FragmentPublicInteractionSurface) { value.Controls[0].AccessibleName = string([]byte{0xff}) }},
		{"control character", func(value *FragmentPublicInteractionSurface) { value.Controls[0].AccessibleName = "Stock\x00query" }},
		{"noncanonical whitespace", func(value *FragmentPublicInteractionSurface) { value.Controls[0].AccessibleName = "Stock  query" }},
		{"encoded entity", func(value *FragmentPublicInteractionSurface) { value.Controls[0].AccessibleName = "Stock &amp; query" }},
		{"empty output name", func(value *FragmentPublicInteractionSurface) {
			value.Outputs[0].AccessibleName = ""
		}},
		{"noncanonical output name", func(value *FragmentPublicInteractionSurface) {
			value.Outputs[0].AccessibleName = "Stock  total"
		}},
		{"duplicate output name", func(value *FragmentPublicInteractionSurface) {
			value.Outputs[1].AccessibleName = value.Outputs[0].AccessibleName
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneFragmentPublicInteractionSurface(base)
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("accepted invalid public interaction surface")
			}
		})
	}
}

func TestFragmentPublicInteractionSurfaceEnforcesBoundsAndPublicShape(t *testing.T) {
	t.Parallel()
	tooManyControls := FragmentPublicInteractionSurface{
		Schema:   FragmentPublicInteractionSurfaceSchemaV1,
		Controls: make([]FragmentPublicInteractionControl, MaxFragmentPublicInteractionControls+1),
	}
	if err := tooManyControls.Validate(); err == nil {
		t.Fatal("accepted too many controls")
	}
	tooManyOutputs := FragmentPublicInteractionSurface{
		Schema:  FragmentPublicInteractionSurfaceSchemaV1,
		Outputs: make([]FragmentPublicOutput, MaxFragmentPublicInteractionOutputs+1),
	}
	if err := tooManyOutputs.Validate(); err == nil {
		t.Fatal("accepted too many outputs")
	}
	receiptOverflow := FragmentPublicInteractionSurface{
		Schema:   FragmentPublicInteractionSurfaceSchemaV1,
		Controls: make([]FragmentPublicInteractionControl, MaxFragmentPublicInteractionControls),
	}
	for index := range receiptOverflow.Controls {
		receiptOverflow.Controls[index] = FragmentPublicInteractionControl{
			Role: FragmentPublicRoleTextbox, RoleOrdinal: index + 1,
			RoleCount:       MaxFragmentPublicInteractionControls,
			AccessibleName:  strings.Repeat("a", MaxFragmentPublicInteractionLiteralBytes),
			PlaceholderHint: strings.Repeat("p", MaxFragmentPublicInteractionLiteralBytes),
			ValueKind:       FragmentPublicValueText,
		}
	}
	if err := receiptOverflow.Validate(); err == nil {
		t.Fatal("accepted oversized canonical receipt")
	}
	assertFragmentPublicJSONFields(t, FragmentPublicInteractionSurface{
		Schema: FragmentPublicInteractionSurfaceSchemaV1,
	}, []string{"controls", "outputs", "schema"})
	assertFragmentPublicJSONFields(t, FragmentPublicInteractionControl{
		Role: FragmentPublicRoleButton, RoleOrdinal: 1, RoleCount: 1,
		AccessibleName: "Reserve", PlaceholderHint: "Optional", ValueKind: FragmentPublicValueAction,
	}, []string{"accessible_name", "placeholder_hint", "role", "role_count", "role_ordinal", "value_kind"})
	assertFragmentPublicJSONFields(t, FragmentPublicOutput{
		AccessibleName: "Result",
	}, []string{"accessible_name"})
}

func assertFragmentPublicJSONFields(t *testing.T, value any, wantFields []string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal public surface value: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode public surface value: %v", err)
	}
	gotFields := make([]string, 0, len(fields))
	for field := range fields {
		gotFields = append(gotFields, field)
	}
	if !sameStringSet(gotFields, wantFields) {
		t.Fatalf("public surface fields=%v want only %v", gotFields, wantFields)
	}
}

func cloneFragmentPublicInteractionSurface(
	surface FragmentPublicInteractionSurface,
) FragmentPublicInteractionSurface {
	clone := surface
	clone.Controls = append([]FragmentPublicInteractionControl(nil), surface.Controls...)
	clone.Outputs = append([]FragmentPublicOutput(nil), surface.Outputs...)
	return clone
}

func sameStringSet(left, right []string) bool {
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	return reflect.DeepEqual(leftSet, func() map[string]struct{} {
		set := make(map[string]struct{}, len(right))
		for _, value := range right {
			set[value] = struct{}{}
		}
		return set
	}())
}
