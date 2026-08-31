package modelconfig

import (
	"fmt"

	"github.com/gryph/omnidex/internal/station"
)

// Authority is one immutable runtime model-config snapshot. Its internal map
// is never exposed; callers receive isolated projections.
type Authority struct {
	config Config
}

func Freeze(config Config) (Authority, error) {
	frozen := Config{}
	for key, value := range config {
		if _, ok := definitionForKey(key); !ok {
			return Authority{}, fmt.Errorf("model config contains unsupported field %q", key)
		}
		frozen[key] = value
	}
	return Authority{config: frozen}, nil
}

func (authority Authority) Config() Config {
	return authority.config.Clone()
}

// Resolve applies explicit project overrides to the frozen runtime snapshot.
// The returned config is complete and can be persisted as the sole job-level
// routing authority.
func (authority Authority) Resolve(overrides Config) (Config, error) {
	frozenOverrides, err := Freeze(overrides)
	if err != nil {
		return nil, err
	}
	resolved := authority.Config()
	for key, value := range frozenOverrides.config {
		resolved[key] = value
	}
	return resolved, nil
}

type Routing struct {
	Stations              map[station.ID]string
	RoleplaySemanticModel string
}

// Routing derives the worker projection mechanically from this authority.
func (authority Authority) Routing() Routing {
	routing := Routing{Stations: map[station.ID]string{}}
	for _, definition := range fieldRegistry {
		value, configured := authority.config[definition.Key]
		if !configured {
			continue
		}
		for _, stationID := range definition.Stations {
			routing.Stations[stationID] = value
		}
		if definition.RoleplaySemantic {
			routing.RoleplaySemanticModel = value
		}
	}
	return routing
}
