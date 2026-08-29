package queue

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const roleplayUserTurnContributionKindMigration = "153_roleplay_user_turn_contribution_kind_authority.sql"

func TestRoleplayUserTurnContributionKindMigrationInstallsExactAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + roleplayUserTurnContributionKindMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	normalized := strings.Join(strings.Fields(source), " ")
	for _, required := range []string{
		"LOCK TABLE roleplay_user_turns IN SHARE ROW EXCLUSIVE MODE;",
		"DROP CONSTRAINT roleplay_user_turns_contribution_kind_check,",
		"ADD CONSTRAINT roleplay_user_turns_contribution_kind_authority_check CHECK",
		"inherited roleplay user-turn contribution-kind constraint is absent",
		"pg_get_constraintdef(oid),convalidated",
		"installed_validated IS DISTINCT FROM TRUE",
		"roleplay user-turn contribution-kind authority postcondition failed",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("contribution-kind migration omitted %q", required)
		}
	}
	if strings.Contains(normalized, "DROP CONSTRAINT IF EXISTS") ||
		strings.Contains(normalized, "NOT VALID") {
		t.Fatalf("contribution-kind migration weakens explicit failure or validation: %s", normalized)
	}

	constraint := regexp.MustCompile(
		`(?s)ADD\s+CONSTRAINT\s+roleplay_user_turns_contribution_kind_authority_check\s+CHECK\s*\(\s*contribution_kind\s+IN\s*\((.*?)\)\s*\)\s*;`,
	).FindStringSubmatch(source)
	if len(constraint) != 2 {
		t.Fatal("contribution-kind migration has no exact named CHECK body")
	}
	quoted := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(constraint[1], -1)
	got := make([]string, len(quoted))
	for index, match := range quoted {
		got[index] = match[1]
	}
	want := []string{
		"dialogue", "action", "action_dialogue", "structured_turn",
		"narration", "direction", "narration_direction", "command",
		"legacy_untyped",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("contribution-kind CHECK values=%q want exact ordered set %q", got, want)
	}
}
