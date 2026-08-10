package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMicrogauntletArtifactsSealFullPublicAndPrivateAuthoritiesSeparately(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	publicDirectory := filepath.Join(root, "public")
	privateDirectory := filepath.Join(root, "private")
	if err := os.Mkdir(publicDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(privateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := MicrogauntletArtifactPaths{
		PublicManifest:  filepath.Join(publicDirectory, "manifest.json"),
		PublicScenario:  filepath.Join(publicDirectory, "scenario.json"),
		PrivateManifest: filepath.Join(privateDirectory, "manifest.json"),
		PrivateOracle:   filepath.Join(privateDirectory, "oracle.json"),
	}
	if err := fixture.SealArtifacts(SurfaceFilesystem, paths); err != nil {
		t.Fatal(err)
	}
	publicRaw, err := os.ReadFile(paths.PublicScenario)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"\"seed\"", "\"witness\"", "\"required_evidence\"", "\"task_archetype\""} {
		if strings.Contains(string(publicRaw), forbidden) {
			t.Fatalf("public scenario leaked %q", forbidden)
		}
	}
	privateRaw, err := os.ReadFile(paths.PrivateOracle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(privateRaw), `"seed"`) || !strings.Contains(string(privateRaw), `"witness"`) {
		t.Fatal("private oracle omitted its sealed generator or witness authority")
	}
	if err := fixture.SealArtifacts(SurfaceFilesystem, paths); err == nil {
		t.Fatal("microgauntlet artifacts were overwritten")
	}
}

func TestMicrogauntletArtifactsRejectSharedPublicAndPrivateDirectory(t *testing.T) {
	root := t.TempDir()
	paths := MicrogauntletArtifactPaths{
		PublicManifest:  filepath.Join(root, "public-manifest.json"),
		PublicScenario:  filepath.Join(root, "public-scenario.json"),
		PrivateManifest: filepath.Join(root, "private-manifest.json"),
		PrivateOracle:   filepath.Join(root, "private-oracle.json"),
	}
	if err := validateArtifactPaths(paths); err == nil {
		t.Fatal("public and private microgauntlet artifacts shared one directory")
	}
}
