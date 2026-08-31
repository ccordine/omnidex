package version

import (
	"fmt"
	"strings"
)

var (
	Version      = "v0.5.0"
	Codename     = "Charmeleon"
	SourceSHA256 = ""
	Date         = ""
)

type PrideRelease struct {
	Version    string
	Codename   string
	NationalID int
	Stage      string
}

var PrideLine = []PrideRelease{
	{Version: "v0.1.0-alpha", Codename: "Bulbasaur", NationalID: 1, Stage: "alpha"},
	{Version: "v0.2.0", Codename: "Ivysaur", NationalID: 2, Stage: "growth"},
	{Version: "v0.3.0", Codename: "Venusaur", NationalID: 3, Stage: "planner"},
	{Version: "v0.4.0", Codename: "Charmander", NationalID: 4, Stage: "assembly_line"},
	{Version: "v0.5.0", Codename: "Charmeleon", NationalID: 5, Stage: "current"},
	{Version: "future", Codename: "Charizard", NationalID: 6, Stage: "planned"},
}

func Label() string {
	parts := []string{strings.TrimSpace(Version)}
	if codename := strings.TrimSpace(Codename); codename != "" {
		parts = append(parts, "("+codename+")")
	}
	return strings.Join(parts, " ")
}

func Full() string {
	out := Label()
	if date := strings.TrimSpace(Date); date != "" {
		out += " date=" + date
	}
	return out
}

func JSON() map[string]string {
	return map[string]string{
		"version":            strings.TrimSpace(Version),
		"codename":           strings.TrimSpace(Codename),
		"release_scheme":     "pride-national-dex",
		"national_dex_id":    fmt.Sprintf("%d", NationalDexID(Codename)),
		"next_maturity_name": "Charizard",
		"source_sha256":      strings.TrimSpace(SourceSHA256),
		"date":               strings.TrimSpace(Date),
	}
}

func NationalDexID(codename string) int {
	codename = strings.ToLower(strings.TrimSpace(codename))
	for _, release := range PrideLine {
		if strings.ToLower(release.Codename) == codename {
			return release.NationalID
		}
	}
	return 0
}

func PrintName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "omnidex"
	}
	return fmt.Sprintf("%s %s", name, Full())
}
