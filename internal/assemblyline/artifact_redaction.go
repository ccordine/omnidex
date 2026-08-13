package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const maxArtifactIdentities = 64

var (
	artifactIdentityPattern = regexp.MustCompile(`(?i)(?:\b[a-z0-9_-]+(?:/[a-z0-9_.-]+)+\.[a-z][a-z0-9]{0,15}\b|\b[a-z0-9_-]+(?:\.[a-z0-9_-]+)*\.(?:go|ts|tsx|js|jsx|json|md|yaml|yml|toml|sql|html|css|scss|php|py|rs|java|kt)\b)`)
	opaqueArtifactPattern   = regexp.MustCompile(`^ARTIFACT_[1-9][0-9]*$`)
)

func RedactArtifactIdentities(input string) (string, []ArtifactIdentity, error) {
	matches := artifactIdentityPattern.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return input, nil, nil
	}
	identities := make([]ArtifactIdentity, 0, len(matches))
	tokens := make(map[string]string)
	var redacted strings.Builder
	previous := 0
	for _, match := range matches {
		value := input[match[0]:match[1]]
		token, exists := tokens[value]
		if !exists {
			if len(identities) >= maxArtifactIdentities {
				return "", nil, fmt.Errorf("request exceeds the %d-artifact identity limit", maxArtifactIdentities)
			}
			token = fmt.Sprintf("ARTIFACT_%d", len(identities)+1)
			tokens[value] = token
			identities = append(identities, ArtifactIdentity{Token: token, Value: value})
		}
		redacted.WriteString(input[previous:match[0]])
		redacted.WriteString(token)
		previous = match[1]
	}
	redacted.WriteString(input[previous:])
	return redacted.String(), identities, nil
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
