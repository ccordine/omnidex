package queue

import "errors"

// ErrStaleJobGeneration means a worker attempted to use a step whose authority
// was retired by a later generation of the same job.
var ErrStaleJobGeneration = errors.New("stale job generation")

// ErrStepNotWritable means the current step exists but its lifecycle state no
// longer permits the requested worker mutation.
var ErrStepNotWritable = errors.New("step is not writable")

// ErrInterruptedJobRequiresReplan prevents generic waiting-input feedback from
// consuming the canonical boundary created by an explicit interruption.
var ErrInterruptedJobRequiresReplan = errors.New("interrupted job requires explicit replan")

// ErrStaleStepAttempt means a worker write did not carry the exact current,
// unexpired execution-attempt authority for its job generation and step.
var ErrStaleStepAttempt = errors.New("stale step attempt")

// ErrContextProjectionBudget means legacy step context exceeded the hard
// model-visible item or byte ceiling. Context is never silently truncated.
var ErrContextProjectionBudget = errors.New("context projection budget exceeded")
