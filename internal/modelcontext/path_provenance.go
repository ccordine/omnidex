package modelcontext

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ArtifactIdentityProvenance is a validated code-owned current-tree view. It
// grants identity only to exact normalized relative paths and to basenames
// that resolve to exactly one such path.
type ArtifactIdentityProvenance struct {
	paths           []string
	mentions        map[string]string
	orderedMentions []string
}

func NewArtifactIdentityProvenance(paths []string) (ArtifactIdentityProvenance, error) {
	known := append([]string(nil), paths...)
	sort.Strings(known)
	mentions := make(map[string]string, len(known)*2)
	basenameOwners := make(map[string][]string, len(known))
	previous := ""
	for index, value := range known {
		if err := validateProvenancePath(value); err != nil {
			return ArtifactIdentityProvenance{}, fmt.Errorf(
				"artifact identity provenance path %d: %w", index, err,
			)
		}
		if index > 0 && value == previous {
			return ArtifactIdentityProvenance{}, fmt.Errorf(
				"artifact identity provenance path %q is duplicated", value,
			)
		}
		previous = value
		mentions[value] = value
		base := path.Base(value)
		basenameOwners[base] = append(basenameOwners[base], value)
	}
	for base, owners := range basenameOwners {
		if len(owners) == 1 {
			mentions[base] = owners[0]
		}
	}
	orderedMentions := make([]string, 0, len(mentions))
	for mention := range mentions {
		orderedMentions = append(orderedMentions, mention)
	}
	sort.Slice(orderedMentions, func(left, right int) bool {
		if len(orderedMentions[left]) != len(orderedMentions[right]) {
			return len(orderedMentions[left]) > len(orderedMentions[right])
		}
		return orderedMentions[left] < orderedMentions[right]
	})
	return ArtifactIdentityProvenance{
		paths: known, mentions: mentions, orderedMentions: orderedMentions,
	}, nil
}

func (provenance ArtifactIdentityProvenance) Paths() []string {
	return append([]string(nil), provenance.paths...)
}

func (provenance ArtifactIdentityProvenance) resolve(value string) (string, bool) {
	if len(provenance.mentions) == 0 {
		return "", false
	}
	if resolved, exists := provenance.mentions[value]; exists {
		return resolved, true
	}
	if strings.HasPrefix(value, "./") {
		resolved, exists := provenance.mentions[strings.TrimPrefix(value, "./")]
		return resolved, exists
	}
	return "", false
}

func (provenance ArtifactIdentityProvenance) identities(value string) []PathIdentity {
	identities := make([]PathIdentity, 0)
	for _, mention := range provenance.orderedMentions {
		for offset := 0; offset <= len(value)-len(mention); {
			relative := strings.Index(value[offset:], mention)
			if relative < 0 {
				break
			}
			start := offset + relative
			end := start + len(mention)
			offset = end
			if !artifactMentionBoundary(value, start, end) ||
				pathIdentityOverlap(start, end, identities) {
				continue
			}
			identities = append(identities, PathIdentity{
				Start: start, End: end, Value: provenance.mentions[mention],
			})
		}
	}
	return identities
}

func artifactMentionBoundary(value string, start, end int) bool {
	if start > 0 {
		previous := value[start-1]
		if artifactMentionByte(previous) ||
			(previous == '.' && start > 1 && artifactMentionByte(value[start-2])) {
			return false
		}
	}
	if end < len(value) {
		next := value[end]
		if artifactMentionByte(next) ||
			(next == '.' && end+1 < len(value) && artifactMentionByte(value[end+1])) {
			return false
		}
	}
	return true
}

func artifactMentionByte(value byte) bool {
	return isASCIIAlpha(value) || value >= '0' && value <= '9' ||
		value == '_' || value == '-' || value == '/' || value == '\\'
}

func pathIdentityOverlap(start, end int, identities []PathIdentity) bool {
	for _, identity := range identities {
		if start < identity.End && end > identity.Start {
			return true
		}
	}
	return false
}

func validateProvenancePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') ||
		strings.Contains(value, `\`) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("%q must be one normalized relative slash path", value)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%q must be one normalized relative slash path", value)
	}
	return nil
}
