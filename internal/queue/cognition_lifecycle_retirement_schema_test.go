package queue

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCognitionLifecycleRetirementMigrationOwnsExactAuthority(t *testing.T) {
	t.Parallel()
	base := readCognitionMigration(t, "058_cognition_lifecycle_retirement.sql")
	guards := readCognitionMigration(t, "058_cognition_lifecycle_retirement_guards.sql")
	for _, required := range []string{
		"ALTER COLUMN authority_kind DROP DEFAULT",
		"authority_kind='worker' AND sealed_attempt IS NOT NULL",
		"authority_kind='lifecycle' AND sealed_attempt IS NULL",
		"CREATE TABLE cognition_lifecycle_retirements",
		"CREATE TABLE cognition_lifecycle_operation_seals",
		"CREATE TABLE cognition_lifecycle_operation_seal_episodes",
		"cognition_json_object_has_exact_keys",
		"cognition_json_array_objects_have_exact_keys",
		"identity_json::jsonb=jsonb_set",
		"prevent_cognition_immutable_mutation",
	} {
		if !strings.Contains(base, required) {
			t.Fatalf("lifecycle retirement migration omitted %q", required)
		}
	}
	for _, required := range []string{
		"cognition_lifecycle_retirement_exact",
		"steps.superseded_at_generation=retirements.job_generation+1",
		"attempts.status='superseded'",
		"cognition_lifecycle_seal_set_exact(NEW.operation_id)",
		"cognition lifecycle retirement % is absent from complete operation seal set %",
		"job_lifecycle_operations_require_cognition_seals",
		"cognition_lifecycle_seal_episodes_exact",
	} {
		if !strings.Contains(guards, required) {
			t.Fatalf("lifecycle retirement guard migration omitted %q", required)
		}
	}
}

func TestCognitionLifecycleRetirementHasOneProductionAuthority(t *testing.T) {
	t.Parallel()
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	retirementOwners := make([]string, 0, 3)
	persistenceOwners := make([]string, 0, 1)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") ||
			strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		if strings.Contains(source, "retireCognitionEpisodesForLifecycleTx(") {
			retirementOwners = append(retirementOwners, file.Name())
		}
		if strings.Contains(source, "INSERT INTO cognition_lifecycle_retirements") {
			persistenceOwners = append(persistenceOwners, file.Name())
		}
	}
	sort.Strings(retirementOwners)
	sort.Strings(persistenceOwners)
	wantRetirement := []string{
		"cognition_lifecycle_retirement_store.go", "repository_cancel.go", "repository_replan.go",
	}
	if !reflect.DeepEqual(retirementOwners, wantRetirement) {
		t.Fatalf("alternate/missing lifecycle retirement authorities=%v", retirementOwners)
	}
	if !reflect.DeepEqual(persistenceOwners, []string{"cognition_lifecycle_retirement_persist.go"}) {
		t.Fatalf("alternate lifecycle retirement persistence=%v", persistenceOwners)
	}
}

func TestCognitionLifecycleRetirementFilesRemainFocused(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("cognition_lifecycle*.go")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths,
		"cognition_canceled_progress.go", "cognition_terminal_persist.go",
		"cognition_terminal_working_set.go", "../cognitionruntime/canceled_recovery.go",
		"../../migrations/058_cognition_lifecycle_retirement.sql",
		"../../migrations/058_cognition_lifecycle_retirement_guards.sql",
	)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines++
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			t.Fatal(err)
		}
		file.Close()
		if lines > 300 {
			t.Fatalf("lifecycle retirement file %s has %d lines; split it", path, lines)
		}
	}
}
