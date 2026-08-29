package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

func mustCanonicalJSON(value any) string {
	raw, err := exactjson.Canonical(value)
	if err != nil {
		panic(fmt.Sprintf("canonical queue authority: %v", err))
	}
	return string(raw)
}
