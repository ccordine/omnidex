package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestTestingLibraryRoleObservationProjectsExactCurrentAccessibleName(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]struct {
		role, matcher, currentName string
	}{
		"parcel action": {role: "button", matcher: `/dispatch parcels/i`, currentName: "7 ready"},
		"room reading":  {role: "heading", matcher: `/room temperature/i`, currentName: "21 °C"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			feedback, err := directCodingTypeScriptStructuredTestModelFailure(
				directCodingVitestFailureEvidence{
					Name: "TestingLibraryElementError",
					Message: "Unable to find an accessible element with the role \"" +
						fixture.role + "\" and name `" + fixture.matcher + "`.\n\nprovider DOM",
					AccessibilityObservation: completeTestingLibraryRoleObservation(
						fixture.role, directCodingTestingLibraryRoleVisibilityAccessible,
						fixture.currentName,
					),
				},
				assemblyline.ArtifactIdentityProvenance{}, fixture.matcher,
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"plain text", "Runtime accessibility observation:",
				"one element with computed accessible name exact text " +
					`"` + fixture.currentName + `"`,
			} {
				if !strings.Contains(feedback, required) {
					t.Fatalf("feedback omitted %q: %q", required, feedback)
				}
			}
			if strings.Contains(feedback, fixture.matcher) || strings.Contains(feedback, "provider DOM") ||
				modelcontext.ContainsPathIdentity(feedback) {
				t.Fatalf("feedback retained unsafe provider data: %q", feedback)
			}
		})
	}
}

func TestTestingLibraryRoleObservationCountAndVisibilityAreExact(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]struct {
		message    string
		visibility directCodingTestingLibraryRoleVisibility
		names      []string
		required   []string
	}{
		"no accessible buttons": {
			message:    "Unable to find an accessible element with the role \"button\"",
			visibility: directCodingTestingLibraryRoleVisibilityAccessible,
			required:   []string{"currently has zero elements"},
		},
		"two available buttons": {
			message:    "Unable to find an element with the role \"button\" and name \"Publish\"",
			visibility: directCodingTestingLibraryRoleVisibilityAvailable,
			names:      []string{"Save", "Cancel"},
			required: []string{
				"currently has 2 elements", `exact text "Save"; exact text "Cancel"`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			feedback, err := directCodingTypeScriptStructuredTestModelFailure(
				directCodingVitestFailureEvidence{
					Name: "TestingLibraryElementError", Message: fixture.message,
					AccessibilityObservation: completeTestingLibraryRoleObservation(
						"button", fixture.visibility, fixture.names...,
					),
				},
				assemblyline.ArtifactIdentityProvenance{},
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range fixture.required {
				if !strings.Contains(feedback, required) {
					t.Fatalf("feedback omitted %q: %q", required, feedback)
				}
			}
		})
	}
}

func TestTestingLibraryRoleObservationEncodesPathShapedNames(t *testing.T) {
	t.Parallel()
	const unsafe = `/tmp/private.ts and /restock/i with "quotes" and \\escapes`
	feedback, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{
			Name:    "TestingLibraryElementError",
			Message: "Unable to find an accessible element with the role \"button\" and name \"safe\"",
			AccessibilityObservation: completeTestingLibraryRoleObservation(
				"button", directCodingTestingLibraryRoleVisibilityAccessible, unsafe,
			),
		},
		assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/tmp/", "private.ts", "/restock/i", unsafe} {
		if strings.Contains(feedback, forbidden) {
			t.Fatalf("feedback retained path-shaped name %q: %q", forbidden, feedback)
		}
	}
	for _, required := range []string{
		"exact UTF-8 byte sequence", "forward-slash (solidus) character (U+002F)",
		"backslash (reverse solidus) character (U+005C)",
	} {
		if !strings.Contains(feedback, required) {
			t.Fatalf("feedback omitted lossless encoding %q: %q", required, feedback)
		}
	}
	if modelcontext.ContainsPathIdentity(feedback) {
		t.Fatalf("feedback retained path identity: %q", feedback)
	}
}

func TestTestingLibraryRoleObservationFailsLoudlyWhenMissingOrContradictory(t *testing.T) {
	t.Parallel()
	base := directCodingVitestFailureEvidence{
		Name:    "TestingLibraryElementError",
		Message: "Unable to find an accessible element with the role \"button\" and name \"Save\"",
	}
	fixtures := map[string]*directCodingTestingLibraryRoleObservation{
		"missing": nil,
		"wrong role": completeTestingLibraryRoleObservation(
			"heading", directCodingTestingLibraryRoleVisibilityAccessible, "Current",
		),
		"wrong visibility": completeTestingLibraryRoleObservation(
			"button", directCodingTestingLibraryRoleVisibilityAvailable, "Current",
		),
		"limit exceeded": {
			Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
			RequestedRole: "button", Visibility: directCodingTestingLibraryRoleVisibilityAccessible,
			Status:       directCodingTestingLibraryRoleObservationStatusLimitExceeded,
			ElementCount: 101,
		},
		"capture failed": {
			Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
			RequestedRole: "button", Visibility: directCodingTestingLibraryRoleVisibilityAccessible,
			Status: directCodingTestingLibraryRoleObservationStatusCaptureFailed,
		},
	}
	for name, observation := range fixtures {
		t.Run(name, func(t *testing.T) {
			failure := base
			failure.AccessibilityObservation = observation
			if _, err := directCodingTypeScriptStructuredTestModelFailure(
				failure, assemblyline.ArtifactIdentityProvenance{},
			); err == nil {
				t.Fatal("accepted missing or contradictory Testing Library observation")
			}
		})
	}
}

func completeTestingLibraryRoleObservation(
	role string,
	visibility directCodingTestingLibraryRoleVisibility,
	names ...string,
) *directCodingTestingLibraryRoleObservation {
	return &directCodingTestingLibraryRoleObservation{
		Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
		RequestedRole: role, Visibility: visibility,
		Status:       directCodingTestingLibraryRoleObservationStatusComplete,
		ElementCount: int64(len(names)), Names: append([]string(nil), names...),
	}
}
