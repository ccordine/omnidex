package version

import (
	"os"
	"strings"
	"testing"
)

func TestCurrentReleaseIsCharmander(t *testing.T) {
	if Version != "v0.4.0" || Codename != "Charmander" {
		t.Fatalf("current release=%s %s", Version, Codename)
	}
	if NationalDexID(Codename) != 4 {
		t.Fatalf("Charmander National Dex id=%d", NationalDexID(Codename))
	}
	metadata := JSON()
	if metadata["next_maturity_name"] != "Charmeleon" {
		t.Fatalf("next maturity=%q", metadata["next_maturity_name"])
	}
	if len(PrideLine) < 5 || PrideLine[2].Stage == "current" || PrideLine[3].Stage != "current" {
		t.Fatalf("release stages=%+v", PrideLine)
	}
}

func TestReleaseBuilderDefaultsToCharmander(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, required := range []string{`VERSION="v0.4.0"`, `CODENAME="Charmander"`} {
		if !strings.Contains(script, required) {
			t.Fatalf("release builder omitted %s", required)
		}
	}
	for _, forbidden := range []string{`VERSION="v0.3.0"`, `CODENAME="Venusaur"`} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("release builder retained old default %s", forbidden)
		}
	}
}
