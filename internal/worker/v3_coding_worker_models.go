package worker

import (
	"fmt"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func portableModelScope(kind assemblyline.WorkKind) (string, error) {
	return assemblyline.PortableWorkerScopeForWorkKind(kind)
}

func (s *directCodingSession) workerModel(id station.ID) (string, error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.claim == nil {
		return "", fmt.Errorf("direct coding worker model routing is unavailable")
	}
	routing, err := s.runtime.modelRouting()
	if err != nil {
		return "", err
	}
	return stationModel(routing, id)
}
