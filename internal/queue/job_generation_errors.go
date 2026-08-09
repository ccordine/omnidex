package queue

import "errors"

// ErrStaleJobGeneration means a worker attempted to use a step whose authority
// was retired by a later generation of the same job.
var ErrStaleJobGeneration = errors.New("stale job generation")

// ErrStepNotWritable means the current step exists but its lifecycle state no
// longer permits the requested worker mutation.
var ErrStepNotWritable = errors.New("step is not writable")

// ErrStepLeaseRequired prevents reusing a running step identity until worker
// writes carry a monotonically increasing execution-attempt lease.
var ErrStepLeaseRequired = errors.New("step execution-attempt lease is required")

// ErrContextProjectionBudget means legacy step context exceeded the hard
// model-visible item or byte ceiling. Context is never silently truncated.
var ErrContextProjectionBudget = errors.New("context projection budget exceeded")
