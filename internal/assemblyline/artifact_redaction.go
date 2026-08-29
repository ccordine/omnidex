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
	opaqueArtifactPattern        = regexp.MustCompile(`^ARTIFACT_[1-9][0-9]*$`)
	opaqueArtifactMentionPattern = regexp.MustCompile(`ARTIFACT_[1-9][0-9]*`)
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

// RestoreArtifactIdentities applies only code-owned identity bindings after a
// path-free semantic result has been validated. Unknown artifact tokens fail
// explicitly instead of becoming user-visible or guessed filesystem state.
func RestoreArtifactIdentities(
	input string,
	identities []ArtifactIdentity,
) (string, error) {
	resolved, err := artifactIdentityMap(identities)
	if err != nil {
		return "", err
	}
	matches := opaqueArtifactMentionPattern.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var restored strings.Builder
	previous := 0
	for _, match := range matches {
		if !artifactTokenBoundary(input, match[0], match[1]) {
			continue
		}
		token := input[match[0]:match[1]]
		value, exists := resolved[token]
		if !exists {
			return "", fmt.Errorf("semantic result contains unknown artifact token %s", token)
		}
		restored.WriteString(input[previous:match[0]])
		restored.WriteString(value)
		previous = match[1]
	}
	if previous == 0 {
		return input, nil
	}
	restored.WriteString(input[previous:])
	return restored.String(), nil
}

func artifactTokenBoundary(value string, start, end int) bool {
	return (start == 0 || !artifactTokenByte(value[start-1])) &&
		(end == len(value) || !artifactTokenByte(value[end]))
}

func artifactTokenByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
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

// ValidatePathFreeRepairInstructionModelContext validates the mixed grammar of
// one imperative repair instruction. Prose retains the strict path boundary;
// quoted source literals may contain ordinary source escapes, which are
// decoded before their semantic content is checked for filesystem identity.
func ValidatePathFreeRepairInstructionModelContext(label string, values ...string) error {
	return ValidatePathFreeRepairInstructionModelContextWithProvenance(
		label, modelcontext.ArtifactIdentityProvenance{}, values...,
	)
}

func ValidatePathFreeRepairInstructionModelContextWithProvenance(
	label string,
	provenance modelcontext.ArtifactIdentityProvenance,
	values ...string,
) error {
	for index, value := range values {
		matches := modelcontext.RepairInstructionPathIdentities(value, provenance)
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
