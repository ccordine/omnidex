package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func VariantEpisodeRef(
	authority PairedRunAuthority,
	variant Variant,
) (cognition.EpisodeRef, error) {
	public, err := NewPublicRunAuthority(authority, variant)
	if err != nil {
		return cognition.EpisodeRef{}, err
	}
	return PublicVariantEpisodeRef(public)
}

func PublicVariantEpisodeRef(authority PublicRunAuthority) (cognition.EpisodeRef, error) {
	if err := authority.Validate(); err != nil {
		return cognition.EpisodeRef{}, err
	}
	digest, err := authority.SHA256()
	if err != nil {
		return cognition.EpisodeRef{}, fmt.Errorf("derive cognition episode identity: %w", err)
	}
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID("episode-" + digest)}
	return episode, episode.Validate()
}
