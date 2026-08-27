package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedReleaseRejectsInvalidCommitIdentityBeforePublication(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*testing.T, managedReleaseFixture)
		want      string
	}{
		{
			name: "missing manifest",
			configure: func(t *testing.T, fixture managedReleaseFixture) {
				if err := os.Remove(filepath.Join(fixture.release, "RELEASE_COMMIT")); err != nil {
					t.Fatal(err)
				}
			},
			want: "release commit manifest must be a regular non-symlink file",
		},
		{
			name: "padded manifest",
			configure: func(t *testing.T, fixture managedReleaseFixture) {
				commit := strings.Repeat("a", 40)
				writeFixtureFile(t, filepath.Join(fixture.release, "RELEASE_COMMIT"), commit+"\n\n", 0o600)
			},
			want: "must contain exactly one commit line",
		},
		{
			name: "symlink manifest",
			configure: func(t *testing.T, fixture managedReleaseFixture) {
				manifest := filepath.Join(fixture.release, "RELEASE_COMMIT")
				real := filepath.Join(fixture.release, "real-release-commit")
				if err := os.Rename(manifest, real); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, manifest); err != nil {
					t.Fatal(err)
				}
			},
			want: "release commit manifest must be a regular non-symlink file",
		},
		{
			name: "binary mismatch",
			configure: func(t *testing.T, fixture managedReleaseFixture) {
				writeFixtureFile(t, filepath.Join(fixture.release, "bin", "omni"), `#!/usr/bin/env bash
set -euo pipefail
printf '{\n  "commit": "%s"\n}\n' '`+strings.Repeat("b", 40)+`'
`, 0o700)
			},
			want: "release binary reports commit " + strings.Repeat("b", 40),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newManagedReleaseFixture(t)
			test.configure(t, fixture)
			environment := filepath.Join(t.TempDir(), "deployment.env")
			writeFixtureFile(t, environment, "CORE_URL=https://managed.example\n", 0o600)
			output, err := runManagedReleaseInstaller(
				fixture, nil, "--prefix", fixture.target, "--env-file", environment, "--yes",
			)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("identity rejection error=%v output=%q want %q", err, output, test.want)
			}
			if _, statErr := os.Stat(fixture.target); !os.IsNotExist(statErr) {
				t.Fatalf("invalid release identity published target: %v", statErr)
			}
		})
	}
}

func TestReleaseBuilderVerifiesManifestAndBinariesBeforeArchive(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "scripts/build-release.sh")
	manifest := strings.Index(body, `printf '%s\n' "$RELEASE_COMMIT"`)
	verify := strings.Index(body, `release_identity_verify_binaries "$target_dir" "$RELEASE_COMMIT" "$ext"`)
	archive := strings.Index(body, `archive_target "$target_dir"`)
	if manifest < 0 || verify < manifest || archive < verify {
		t.Fatalf("release build identity ordering manifest=%d verify=%d archive=%d", manifest, verify, archive)
	}
	for _, fragment := range []string{
		`"scripts/release-binary-identity-lib.sh"`,
		`chmod 0444 "${target_dir}/${RELEASE_COMMIT_MANIFEST}"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("release builder omits identity fragment %q", fragment)
		}
	}
}
