package assemblyline

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const maxArtifactIdentities = 64

type ArtifactIdentityProvenance = modelcontext.ArtifactIdentityProvenance

var (
	opaqueArtifactPattern = regexp.MustCompile(`^ARTIFACT_[1-9][0-9]*$`)
)

func RedactArtifactIdentities(
	input string,
	provenance modelcontext.ArtifactIdentityProvenance,
) (string, []ArtifactIdentity, error) {
	matches := modelcontext.PathIdentities(input, provenance)
	if len(matches) == 0 {
		return input, nil, nil
	}
	identities := make([]ArtifactIdentity, 0, len(matches))
	tokens := make(map[string]string)
	var redacted strings.Builder
	previous := 0
	for _, match := range matches {
		token, exists := tokens[match.Value]
		if !exists {
			if len(identities) >= maxArtifactIdentities {
				return "", nil, fmt.Errorf("request exceeds the %d-artifact identity limit", maxArtifactIdentities)
			}
			token = fmt.Sprintf("ARTIFACT_%d", len(identities)+1)
			tokens[match.Value] = token
			identities = append(identities, ArtifactIdentity{Token: token, Value: match.Value})
		}
		redacted.WriteString(input[previous:match.Start])
		redacted.WriteString(token)
		previous = match.End
	}
	redacted.WriteString(input[previous:])
	return redacted.String(), identities, nil
}

// ValidatePathFreeModelContext is the final byte-level invariant for untyped
// model-visible prose. Typed HTTP route and media fields are validated by their
// owning contracts and never weaken this general boundary.
func ValidatePathFreeModelContext(label string, values ...string) error {
	return validatePathFreeModelContextWithProvenance(
		label, modelcontext.ArtifactIdentityProvenance{}, values...,
	)
}

func ValidatePathFreeModelContextWithProvenance(
	label string,
	provenance modelcontext.ArtifactIdentityProvenance,
	values ...string,
) error {
	return validatePathFreeModelContextWithProvenance(label, provenance, values...)
}

func validatePathFreeModelContextWithProvenance(
	label string,
	provenance modelcontext.ArtifactIdentityProvenance,
	values ...string,
) error {
	for index, value := range values {
		matches := modelcontext.PathIdentities(value, provenance)
		if len(matches) == 0 {
			continue
		}
		match := matches[0]
		return fmt.Errorf(
			"%s field %d contains filesystem identity %q",
			label, index+1, value[match.Start:match.End],
		)
	}
	return nil
}

// ValidatePathFreeSourceModelContext is restricted to parser-proven source
// nodes. It preserves source grammar such as division and regular expressions,
// while rejecting path-bearing literals, module specifiers, and comments.
func ValidatePathFreeSourceModelContext(label string, values ...string) error {
	return ValidatePathFreeSourceModelContextWithProvenance(
		label, modelcontext.ArtifactIdentityProvenance{}, values...,
	)
}

func ValidatePathFreeSourceModelContextWithProvenance(
	label string,
	provenance modelcontext.ArtifactIdentityProvenance,
	values ...string,
) error {
	for index, value := range values {
		matches := modelcontext.SourcePathIdentities(value, provenance)
		if len(matches) == 0 {
			continue
		}
		match := matches[0]
		return fmt.Errorf(
			"%s source field %d contains filesystem identity %q",
			label, index+1, value[match.Start:match.End],
		)
	}
	return nil
}

func artifactIdentityMap(identities []ArtifactIdentity) (map[string]string, error) {
	resolved := make(map[string]string, len(identities))
	for _, identity := range identities {
		if !opaqueArtifactPattern.MatchString(identity.Token) || strings.TrimSpace(identity.Value) == "" {
			return nil, fmt.Errorf("invalid opaque artifact identity %#v", identity)
		}
		if _, exists := resolved[identity.Token]; exists {
			return nil, fmt.Errorf("duplicate opaque artifact token %s", identity.Token)
		}
		resolved[identity.Token] = identity.Value
	}
	return resolved, nil
}
