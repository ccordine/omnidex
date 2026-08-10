package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflinePromotionRejectsNestedRelativeAndSymlinkOutputAuthority(t *testing.T) {
	base := validOfflineOutputConfig(t)
	tests := []struct {
		name   string
		mutate func(*OfflinePromotionConfig)
		want   string
	}{
		{name: "relative public", mutate: func(config *OfflinePromotionConfig) {
			config.PublicOutputDirectory = "relative-public"
		}, want: "unavailable or inexact"},
		{name: "public contains private", mutate: func(config *OfflinePromotionConfig) {
			private := filepath.Join(config.PublicOutputDirectory, "private")
			mustPrivateDirectory(t, private)
			config.PrivateOutputDirectory = private
		}, want: "cannot contain"},
		{name: "private contains public", mutate: func(config *OfflinePromotionConfig) {
			public := filepath.Join(config.PrivateOutputDirectory, "public")
			if err := os.Mkdir(public, 0o700); err != nil {
				t.Fatal(err)
			}
			config.PublicOutputDirectory = public
		}, want: "cannot contain"},
		{name: "symlink public", mutate: func(config *OfflinePromotionConfig) {
			link := filepath.Join(t.TempDir(), "public-link")
			if err := os.Symlink(config.PublicOutputDirectory, link); err != nil {
				t.Fatal(err)
			}
			config.PublicOutputDirectory = link
		}, want: "unavailable or inexact"},
		{name: "shared private mode", mutate: func(config *OfflinePromotionConfig) {
			if err := os.Chmod(config.PrivateOutputDirectory, 0o750); err != nil {
				t.Fatal(err)
			}
		}, want: "exclusive owner authority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.mutate(&config)
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("output authority error=%v want=%q", err, test.want)
			}
		})
	}
}

func TestOfflinePromotionRejectsSymlinkOutputTarget(t *testing.T) {
	config := validOfflineOutputConfig(t)
	if err := os.Symlink(
		filepath.Join(t.TempDir(), "missing"), config.Paths().Episode,
	); err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("symlink output error=%v", err)
	}
}

func validOfflineOutputConfig(t *testing.T) OfflinePromotionConfig {
	t.Helper()
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider, host := offlinePrepareTestAttestations(t)
	prepared, err := prepareOfflineExperiment(
		request, provider, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared.promotion
}

func mustPrivateDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
