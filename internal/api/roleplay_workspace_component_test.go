package api

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayWorkspaceComponentsRenderPaginatedServerAuthority(t *testing.T) {
	now := time.Now().UTC()
	worldNext := true
	worldPage, err := renderRoleplayWorldPage(roleplay.WorldPage{
		Offset: 0, HasMore: worldNext,
		Items: []roleplay.WorldSummary{{
			World: roleplay.World{
				ID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ChannelID: "story-world",
				Name: "Clockwork Harbor", Authority: roleplay.AuthorityFictionalCanon, CreatedAt: now,
			},
			SceneTitle: "Lighthouse Signal", CharacterCount: 2,
		}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		`data-recyclr-target="roleplay-world-list"`, `data-action="chat#selectRoleplayWorld"`,
		`data-channel-id="story-world"`, `Clockwork Harbor`, `Lighthouse Signal`,
		`data-action="chat#loadMoreRoleplayWorlds"`,
	} {
		if !strings.Contains(worldPage.HTML.Bundle, exact) {
			t.Fatalf("world bundle omitted %q: %s", exact, worldPage.HTML.Bundle)
		}
	}

	libraryPage, err := renderRoleplayLibraryPage(roleplay.LibraryCharacterPage{
		SelectedWorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Items: []roleplay.LibraryCharacterSummary{{
			ID: "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Mira",
			Authority:   roleplay.AuthorityCharacterIdentity,
			Profile:     &roleplay.PersonaSheet{Summary: "A patient clockmaker."},
			MemoryCount: 3, PlacementCount: 2, CreatedAt: now, UpdatedAt: now,
		}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		`data-recyclr-target="roleplay-library-list"`, `data-action="chat#placeRoleplayCharacter"`,
		`data-library-character-id="rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`3 memories · 2 worlds`,
	} {
		if !strings.Contains(libraryPage.HTML.Bundle, exact) {
			t.Fatalf("library bundle omitted %q: %s", exact, libraryPage.HTML.Bundle)
		}
	}
	if strings.Contains(libraryPage.HTML.Bundle, "innerHTML =") {
		t.Fatal("server component contains a JavaScript renderer")
	}
}

func TestRoleplayLibraryComponentDoesNotOfferInvalidPlacement(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	character := roleplay.LibraryCharacterSummary{
		ID: "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Mira",
		Authority: roleplay.AuthorityCharacterIdentity, CreatedAt: now, UpdatedAt: now,
	}
	for _, fixture := range []struct {
		name      string
		page      roleplay.LibraryCharacterPage
		want      string
		forbidden string
	}{
		{
			name: "already placed",
			page: roleplay.LibraryCharacterPage{
				SelectedWorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Items: []roleplay.LibraryCharacterSummary{func() roleplay.LibraryCharacterSummary {
					value := character
					value.PlacedInSelectedWorld = true
					return value
				}()},
			},
			want:      `disabled aria-disabled="true">In world</button>`,
			forbidden: `data-action="chat#placeRoleplayCharacter"`,
		},
		{
			name:      "no selected world",
			page:      roleplay.LibraryCharacterPage{Items: []roleplay.LibraryCharacterSummary{character}},
			want:      `disabled aria-disabled="true">Select world</button>`,
			forbidden: `data-action="chat#placeRoleplayCharacter"`,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			component, err := renderRoleplayLibraryPage(fixture.page, 20)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(component.HTML.Bundle, fixture.want) ||
				strings.Contains(component.HTML.Bundle, fixture.forbidden) {
				t.Fatalf("library action state=%s", component.HTML.Bundle)
			}
		})
	}
}
