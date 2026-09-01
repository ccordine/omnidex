package llm

import "time"

// MaximumModelRequestDuration is the hard wall-clock limit for one provider
// request. A caller may choose a shorter timeout, but no model request may run
// beyond this duration.
const MaximumModelRequestDuration = 30 * time.Minute

// BoundedModelRequestDuration applies the hard provider-request ceiling. The
// production configuration path rejects non-positive and over-ceiling values;
// this boundary also protects direct clients that bypass that configuration.
func BoundedModelRequestDuration(requested time.Duration) time.Duration {
	if requested <= 0 || requested > MaximumModelRequestDuration {
		return MaximumModelRequestDuration
	}
	return requested
}
