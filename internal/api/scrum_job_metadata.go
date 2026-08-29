package api

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/scrum"
)

func scrumReturnColumnFromMetadata(raw json.RawMessage) string {
	meta, err := scrum.DecodeStoredJobMetadata(raw)
	if err != nil {
		return ""
	}
	return meta.ReturnColumn
}
