package version

import (
	"os"
	"strings"
	"testing"
)

func TestCurrentReleaseIsCharmeleon(t *testing.T) {
	if Version != "v0.5.0" || Codename != "Charmeleon" {
		t.Fatalf("current release=%s %s", Version, Codename)
	}
	if NationalDexID(Codename) != 5 {
		t.Fatalf("Charmeleon National Dex id=%d", NationalDexID(Codename))
	}
	metadata := JSON()
	if metadata["next_maturity_name"] != "Charizard" {
		t.Fatalf("next maturity=%q", metadata["next_maturity_name"])
	}
	if len(PrideLine) < 6 || PrideLine[3].Stage == "current" || PrideLine[4].Stage != "current" {
		t.Fatalf("release stages=%+v", PrideLine)
	}
}

func TestReleaseBuilderDefaultsToCharmeleon(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, required := range []string{`VERSION="v0.5.0"`, `CODENAME="Charmeleon"`} {
		if !strings.Contains(script, required) {
			t.Fatalf("release builder omitted %s", required)
		}
	}
	for _, forbidden := range []string{`VERSION="v0.4.0"`, `CODENAME="Charmander"`} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("release builder retained old default %s", forbidden)
		}
	}
}
