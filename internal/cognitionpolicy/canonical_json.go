package cognitionpolicy

import "github.com/gryph/omnidex/internal/exactjson"

// canonicalPolicyJSON is the package-local authority seam. The sole JSON
// canonicalizer lives in exactjson and mirrors cognition_canonical_jsonb.
func canonicalPolicyJSON(value any) ([]byte, error) {
	return exactjson.Canonical(value)
}
