package queue

import (
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestEnqueueStructuredRoleplayTurnReturnsExactPersistedParts(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		upgrade bool
	}{
		{name: "fresh"},
		{name: "upgrade from 152", upgrade: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := t.Context()
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if fixture.upgrade {
				if err := repository.EnsureSchema(
					ctx, loadMigrationBundleThroughPrefix(t, "152"),
				); err != nil {
					t.Fatal(err)
				}
				assertRoleplayContributionKindConstraints(t, repository, true, false)
			}
			if err := repository.EnsureSchema(
				ctx, loadMigrationBundleThroughPrefix(t, "153"),
			); err != nil {
				t.Fatal(err)
			}
			assertRoleplayContributionKindConstraints(t, repository, false, true)

			channel, _, actor := setupUserCanonDatabaseChannel(
				t, repository, "structured-receipt", "Structured receipt",
			)
			request := roleplay.UserTurnRequest{
				PersonaKind: roleplay.UserPersonaCharacter, CharacterID: actor.ID,
				ContributionKind: roleplay.UserContributionStructured,
				Parts: []roleplay.UserTurnPart{
					{Kind: roleplay.UserTurnPartAction, Text: "I raise the brass lantern."},
					{Kind: roleplay.UserTurnPartEvent, Text: "Blue fire fills its glass."},
					{Kind: roleplay.UserTurnPartMessage, Text: "Keep close."},
				},
			}
			exact, err := roleplay.ComposeUserTurn(request)
			if err != nil {
				t.Fatal(err)
			}
			message, job, err := repository.EnqueueRoleplayChannelTurn(
				ctx, channel.ID, exact, request,
			)
			if err != nil {
				t.Fatal(err)
			}
			want := modelRoleplayReceiptParts(request.Parts)
			assertExactStructuredRoleplayReceipt(t, message, job, exact, actor.ID, want)

			page, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Messages) != 1 {
				t.Fatalf("persisted structured transcript=%#v", page.Messages)
			}
			assertExactStructuredRoleplayReceipt(
				t, page.Messages[0], job, exact, actor.ID, want,
			)
		})
	}
}

func assertRoleplayContributionKindConstraints(
	t *testing.T,
	repository *Repository,
	wantInherited bool,
	wantAuthority bool,
) {
	t.Helper()
	var inherited, authority bool
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT EXISTS (
		           SELECT 1 FROM pg_constraint
		           WHERE conrelid='roleplay_user_turns'::regclass
		             AND conname='roleplay_user_turns_contribution_kind_check'
		       ),
		       EXISTS (
		           SELECT 1 FROM pg_constraint
		           WHERE conrelid='roleplay_user_turns'::regclass
		             AND conname='roleplay_user_turns_contribution_kind_authority_check'
		             AND contype='c' AND convalidated
		       )
	`).Scan(&inherited, &authority); err != nil {
		t.Fatal(err)
	}
	if inherited != wantInherited || authority != wantAuthority {
		t.Fatalf(
			"contribution-kind constraints inherited/authority=%t/%t want %t/%t",
			inherited, authority, wantInherited, wantAuthority,
		)
	}
}

func modelRoleplayReceiptParts(
	parts []roleplay.UserTurnPart,
) []model.ChannelMessageRoleplayPart {
	result := make([]model.ChannelMessageRoleplayPart, len(parts))
	for index, part := range parts {
		result[index] = model.ChannelMessageRoleplayPart{
			Kind: string(part.Kind), Text: part.Text,
		}
	}
	return result
}

func assertExactStructuredRoleplayReceipt(
	t *testing.T,
	message model.ChannelMessage,
	job model.Job,
	exact string,
	actorID string,
	want []model.ChannelMessageRoleplayPart,
) {
	t.Helper()
	if message.Content != exact || job.Instruction != exact || message.Roleplay == nil ||
		message.Roleplay.PersonaKind != string(roleplay.UserPersonaCharacter) ||
		message.Roleplay.CharacterID != model.RoleplayCharacterID(actorID) ||
		message.Roleplay.ContributionKind != string(roleplay.UserContributionStructured) ||
		!slices.Equal(message.Roleplay.Parts, want) {
		t.Fatalf("structured roleplay receipt message=%#v job=%#v want parts=%#v", message, job, want)
	}
}
