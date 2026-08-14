package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelContextAdmissionDoesNotTreatBytesAsTokens(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	forbidden := []string{
		"ModelInputTokenUpperBound",
		"ValidateInferenceBudget",
		"InferenceInputByteBudget",
		"ValidateExactPreparedInputBudget",
		"Each UTF-8 byte is conservatively treated as one token",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(
		path string, entry fs.DirEntry, walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("retired byte-as-token authority %q remains in %s", token, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for path, required := range map[string][]string{
		"exact_prepared_request.go": {"MaxExactPreparedModelInputBytes", "ValidateExactPreparedNativeUsage"},
		"exact_profile_request.go":  {"Truncate: false"},
	} {
		raw, err := os.ReadFile(filepath.Join(root, "internal", "llm", path))
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range required {
			if !strings.Contains(string(raw), token) {
				t.Errorf("exact provider context authority %s omits %q", path, token)
			}
		}
	}
}
